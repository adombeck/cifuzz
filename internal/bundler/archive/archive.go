package archive

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type ArchiveWriter interface {
	Close() error
	WriteFile(string, string) error
	WriteDir(string, string) error
	WriteHardLink(string, string) error
	GetSourcePath(string) string
	HasFileEntry(string) bool
	Headers() []*tar.Header
}

type NullArchiveWriter struct{}

func (w *NullArchiveWriter) Close() error {
	return nil
}
func (w *NullArchiveWriter) WriteFile(string, string) error {
	return nil
}
func (w *NullArchiveWriter) WriteDir(string, string) error {
	return nil
}
func (w *NullArchiveWriter) WriteHardLink(string, string) error {
	return nil
}
func (w *NullArchiveWriter) GetSourcePath(string) string {
	return ""
}
func (w *NullArchiveWriter) HasFileEntry(string) bool {
	return true
}
func (w *NullArchiveWriter) Headers() []*tar.Header {
	return []*tar.Header{}
}

// TarArchiveWriter provides functions to create a gzip-compressed tar archive.
type TarArchiveWriter struct {
	*tar.Writer
	manifest   map[string]string
	headers    []*tar.Header
	gzipWriter *gzip.Writer
}

func NewTarArchiveWriter(w io.Writer, compress bool) *TarArchiveWriter {
	var gzipWriter *gzip.Writer
	var writer *tar.Writer

	if compress {
		gzipWriter = gzip.NewWriter(w)
		writer = tar.NewWriter(gzipWriter)
	} else {
		writer = tar.NewWriter(w)
	}

	return &TarArchiveWriter{
		Writer:     writer,
		manifest:   make(map[string]string),
		gzipWriter: gzipWriter,
	}
}

// Close closes the tar writer and the gzip writer. It does not close
// the underlying io.Writer.
func (w *TarArchiveWriter) Close() error {
	var err error
	err = w.Writer.Close()
	if err != nil {
		return errors.WithStack(err)
	}

	if w.gzipWriter != nil {
		err = w.gzipWriter.Close()
	}

	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// WriteFile writes the contents of sourcePath to the archive, with the
// filename archivePath (so when the archive is extracted, the file will
// be created at archivePath). Symlinks will be followed.
// WriteFile only handles regular files and symlinks.
func (w *TarArchiveWriter) WriteFile(archivePath string, sourcePath string) error {
	if fileutil.IsDir(sourcePath) {
		return errors.Errorf("file is a directory: %s", sourcePath)
	}
	return w.writeFileOrEmptyDir(archivePath, sourcePath)
}

// writeFileOrEmptyDir does the same as WriteFile but doesn't return an
// error when passed a directory. If passed a directory, it creates an
// empty directory at archivePath.
func (w *TarArchiveWriter) writeFileOrEmptyDir(archivePath string, sourcePath string) error {
	// To match the tar specification, which requires forward slashes as path separators,
	// we convert potential windows path separators to forward slashes.
	// Otherwise tars created on Windows will not work correctly on other platforms.
	archivePath = filepath.ToSlash(archivePath)
	existingAbsPath, conflict := w.manifest[archivePath]
	if conflict {
		if existingAbsPath == sourcePath {
			log.Debugf("Skipping file %q, was already added to the archive", sourcePath)
			return nil
		} else {
			return errors.Errorf("archive path %q has two source files: %q and %q", archivePath, existingAbsPath, sourcePath)
		}
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return errors.WithStack(err)
	}

	// Since os.File.Stat() follows symlinks, info will not be of type symlink
	// at this point - no need to pass in a non-empty value for link.
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return errors.WithStack(err)
	}
	header.Name = archivePath
	err = w.WriteHeader(header)
	if err != nil {
		return errors.WithStack(err)
	}
	w.headers = append(w.headers, header)

	if info.IsDir() {
		return nil
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("not a regular file: %s", sourcePath)
	}

	_, err = io.Copy(w.Writer, f)
	if err != nil {
		return errors.Wrapf(err, "failed to add file to archive: %s", sourcePath)
	}

	w.manifest[archivePath] = sourcePath
	return nil
}

// WriteHardLink adds a hard link header to the archive. When the
// archive is extracted, a hard link to target with the name linkname is
// created.
func (w *TarArchiveWriter) WriteHardLink(target string, linkname string) error {
	existingAbsPath, conflict := w.manifest[linkname]
	if conflict {
		return errors.Errorf("conflict for archive path %q: %q and %q", target, existingAbsPath, linkname)
	}

	header := &tar.Header{
		Typeflag: tar.TypeLink,
		Name:     linkname,
		Linkname: target,
	}
	err := w.WriteHeader(header)
	if err != nil {
		return errors.WithStack(err)
	}
	w.manifest[target] = linkname
	return nil
}

// WriteDir traverses sourceDir recursively and writes all regular files
// and symlinks to the archive.
func (w *TarArchiveWriter) WriteDir(archiveBasePath string, sourceDir string) error {
	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		archivePath := filepath.Join(archiveBasePath, relPath)

		// skip self referencing directories
		if relPath == "." && archivePath == "." {
			return nil
		}

		// There is no harm in creating tar entries for empty directories, even though they are not necessary.
		return w.writeFileOrEmptyDir(archivePath, path)
	})
	if err != nil {
		return errors.WithMessagef(err, "Failed to write files from %s to archive path %s", sourceDir, archiveBasePath)
	}

	return nil
}

func (w *TarArchiveWriter) GetSourcePath(archivePath string) string {
	return w.manifest[archivePath]
}

func (w *TarArchiveWriter) HasFileEntry(archivePath string) bool {
	_, exists := w.manifest[archivePath]
	return exists
}

func (w *TarArchiveWriter) Headers() []*tar.Header {
	return w.headers
}

// Extract extracts the gzip-compressed tar archive bundle into dir.
func Extract(bundle, dir string) error {
	f, err := os.Open(bundle)
	if err != nil {
		return errors.WithStack(err)
	}
	gr, err := gzip.NewReader(f)
	if err != nil {
		return errors.WithStack(err)
	}
	defer gr.Close()
	return archiveutil.Untar(gr, dir)
}
