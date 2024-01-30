package archiveutil

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// UntarFile extracts a tar archive to a destination directory
func UntarFile(source, dest string) error {
	file, err := os.Open(source)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()
	return Untar(file, dest)
}

// Untar extracts a tar archive to a destination directory
func Untar(r io.Reader, dest string) error {
	hardlinks := make(map[string]string)
	tr := tar.NewReader(r)
	for {
		var header *tar.Header
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.WithStack(err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(filepath.Join(dest, header.Name), 0755)
			if err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeReg:
			filePath := filepath.Join(dest, header.Name)
			err = os.MkdirAll(filepath.Dir(filePath), 0755)
			if err != nil {
				return errors.WithStack(err)
			}
			var file *os.File
			file, err = os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = io.Copy(file, tr)
			_ = file.Close()
			if err != nil {
				return errors.WithStack(err)
			}
		case tar.TypeLink:
			// To be able to create the hard link, the target must
			// already exist, which is not necessarily the case yet, so
			// we store the link and target paths and create the hard
			// links after all other files were extracted
			targetpath := filepath.Join(dest, header.Linkname)
			linkpath := filepath.Join(dest, header.Name)
			hardlinks[linkpath] = targetpath
		default:
			return errors.Errorf("unsupported file type: %d", header.Typeflag)
		}
	}

	// Create the hard links
	for linkpath, targetpath := range hardlinks {
		err := os.Link(targetpath, linkpath)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Unzip extracts a ZIP archive to a destination directory
// Based on: https://stackoverflow.com/a/24792688/2804197
// Original author: https://stackoverflow.com/users/1316499/astockwell
// Original license: CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/)
// In addition to the license of the cifuzz git repository, this function
// is licensed under CC BY-SA 4.0 (https://creativecommons.org/licenses/by-sa/4.0/).
func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return errors.WithStack(err)
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			err = os.MkdirAll(path, f.Mode())
			if err != nil {
				return errors.WithStack(err)
			}
		} else {
			err = os.MkdirAll(filepath.Dir(path), f.Mode())
			if err != nil {
				return errors.WithStack(err)
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return errors.WithStack(err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}
