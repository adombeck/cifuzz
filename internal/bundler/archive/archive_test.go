package archive

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestWriteArchive(t *testing.T) {
	testdataDir := filepath.Join("testdata", "archive_test")
	require.DirExists(t, testdataDir)
	dir := testutil.MkdirTemp(t, "", "write-archive-test-*")
	err := copy.Copy(testdataDir, dir)
	require.NoError(t, err)

	// Create an empty directory to test that WriteArchive handles it - it can't be kept in testdata since Git doesn't
	// allow checking in empty directories.
	err = os.MkdirAll(filepath.Join(dir, "empty_dir"), 0o755)
	require.NoError(t, err)

	// Walk the testdata dir and write all contents to an archive
	archive, err := os.CreateTemp("", "bundle-*.tar.gz")
	require.NoError(t, err)
	t.Cleanup(func() { fileutil.Cleanup(archive.Name()) })
	writer := bufio.NewWriter(archive)
	archiveWriter := NewTarArchiveWriter(writer, true)
	err = archiveWriter.WriteDir("", dir)
	require.NoError(t, err)
	err = archiveWriter.WriteHardLink(filepath.Join("dir1", "dir2", "test.sh"), filepath.Join("dir1", "hardlink"))
	require.NoError(t, err)

	err = archiveWriter.Close()
	require.NoError(t, err)
	err = writer.Flush()
	require.NoError(t, err)
	err = archive.Close()
	require.NoError(t, err)

	// Unpack archive contents with tar.
	out := testutil.MkdirTemp(t, "", "archive-test-*")
	cmd := exec.Command("tar", "-xvf", archive.Name(), "-C", out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("Command: %v", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)

	remainingExpectedEntries := []struct {
		RelPath          string
		FileContent      string
		IsExecutableFile bool
	}{
		{".", "", false},
		{"dir1", "", false},
		{filepath.Join("dir1", "symlink"), "#!/usr/bin/env bash", true},
		{filepath.Join("dir1", "hardlink"), "#!/usr/bin/env bash", true},
		{filepath.Join("dir1", "dir2"), "", false},
		{filepath.Join("dir1", "dir2", "test.sh"), "#!/usr/bin/env bash", true},
		{filepath.Join("dir1", "dir2", "test.txt"), "foobar", false},
		{"empty_dir", "", false},
	}
	// Verify that the archive contains exactly the expected files and directories.
	// Do not assert group and other permissions which may be affected by masks.
	err = filepath.WalkDir(out, func(absPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		relPath, err := filepath.Rel(out, absPath)
		if err != nil {
			return errors.WithStack(err)
		}
		for i, expectedEntry := range remainingExpectedEntries {
			if relPath != expectedEntry.RelPath {
				continue
			}

			shouldBeDir := expectedEntry.FileContent == ""
			isDir := fileutil.IsDir(absPath)
			require.Equalf(t, shouldBeDir, isDir, "Directory/file status doesn't match for %q", relPath)

			if isDir {
				remainingExpectedEntries = append(remainingExpectedEntries[:i], remainingExpectedEntries[i+1:]...)
				return nil
			}

			// Perform additional checks on files.
			stat, err := os.Lstat(absPath)
			require.NoError(t, err)
			require.Falsef(
				t,
				stat.Mode()&os.ModeSymlink == os.ModeSymlink,
				"Expected symlinks to be archived as regular files: %q is a symlink",
				relPath,
			)

			if runtime.GOOS != "windows" {
				shouldBeExecutable := expectedEntry.IsExecutableFile
				isExecutable := stat.Mode()&0o100 == 0o100
				require.Equalf(
					t,
					shouldBeExecutable,
					isExecutable,
					"Expected executable bit to be preserved, unexpected value for %s",
					relPath,
				)
			}

			content, err := os.ReadFile(absPath)
			require.NoError(t, err)
			require.Equalf(t, expectedEntry.FileContent, string(content), "Contents are not as expected: %q", relPath)

			remainingExpectedEntries = append(remainingExpectedEntries[:i], remainingExpectedEntries[i+1:]...)
			return nil
		}
		require.Fail(t, "Unexpected archive content: "+relPath)
		return nil
	})
	require.NoError(t, err)
	var msg strings.Builder
	for _, missingEntry := range remainingExpectedEntries {
		msg.WriteString(fmt.Sprintf("  %q\n", missingEntry.RelPath))
	}
	require.Empty(t, remainingExpectedEntries, "Archive did not contain the following expected entries: %s", msg.String())
}

// Independently from the operating system, path separators in archive files have
// to be always forward slashes.
func TestInternalPaths(t *testing.T) {
	testFile := filepath.Join("testdata", "archive_test", "dir1", "dir2", "test.txt")
	require.FileExists(t, testFile)

	archiveFile := createArchive(t, []fileEntry{
		{filepath.Join("archive-dir", "hello"), testFile},
	})

	// Verify that file header has correct path separators.
	// Unfortunately extracting the archive under Windows
	// with the tar command or the archiveutils.Untar function
	// will not show the actual problem, as it seems there are
	// workarounds already in place.
	archiveRead, err := os.Open(archiveFile.Name())
	require.NoError(t, err)
	t.Cleanup(func() { archiveRead.Close() })

	gr, err := gzip.NewReader(archiveRead)
	require.NoError(t, err)
	t.Cleanup(func() { gr.Close() })

	tr := tar.NewReader(gr)
	header, err := tr.Next()
	require.NoError(t, err)

	require.Equal(t, "archive-dir/hello", header.Name)
}

// TestDuplicateFileContent verifies that the same file content is only stored
// once in the archive. This tests a regression where the same file content was
// stored multiple times, resulting in an unnecessarily large archive.
func TestDuplicateFileContent(t *testing.T) {
	testFile := filepath.Join("testdata", "dummy.blob")
	require.FileExists(t, testFile)

	archiveFile := createArchive(t, []fileEntry{
		{"dummy.blob", testFile},
	})

	archiveStat, err := archiveFile.Stat()
	require.NoError(t, err)

	expectedSize := archiveStat.Size()
	t.Logf("Created archive with size %d", expectedSize)

	// Create a new archive with the same file content multiple times.
	archiveFile = createArchive(t, []fileEntry{
		{"dummy.blob", testFile},
		{"dummy.blob", testFile},
		{"dummy.blob", testFile},
		{"dummy.blob", testFile},
	})

	archiveStat, err = archiveFile.Stat()
	require.NoError(t, err)

	actualSize := archiveStat.Size()
	t.Logf("Created archive with size %d", actualSize)

	require.Equal(t, expectedSize, actualSize)
}

// Use a struct instead of a map to allow multiple entries with the same
// archive / source path.
type fileEntry struct {
	archivePath string
	sourcePath  string
}

// Creates a tar.gz archive with the given files.
func createArchive(t *testing.T, files []fileEntry) *os.File {
	archiveFile, err := os.CreateTemp("", "bundle-*.tar.gz")
	require.NoError(t, err)
	t.Cleanup(func() { fileutil.Cleanup(archiveFile.Name()) })

	writer := bufio.NewWriter(archiveFile)
	archiveWriter := NewTarArchiveWriter(writer, true)

	for _, fileEntry := range files {
		err = archiveWriter.WriteFile(fileEntry.archivePath, fileEntry.sourcePath)
		require.NoError(t, err)
	}

	err = archiveWriter.Close()
	require.NoError(t, err)
	err = writer.Flush()
	require.NoError(t, err)
	t.Cleanup(func() {
		archiveFile.Close()
	})

	t.Logf("Created archive at: %s", archiveFile.Name())
	return archiveFile
}
