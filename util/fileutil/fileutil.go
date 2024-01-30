package fileutil

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
)

func IsSymlink(path string) bool {
	f, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return f.Mode()&os.ModeSymlink != 0
}

// IsDir returns whether this path is a directory. Tries to behave the
// same as Python's pathlib.Path.is_dir()
func IsDir(path string) bool {
	f, err := os.Stat(path)
	if err != nil {
		return false
	}
	return f.Mode()&os.ModeDir != 0
}

// Touch creates a file at the given path
func Touch(path string) error {
	file, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return errors.WithStack(err)
	}
	err = file.Close()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errors.WithStack(err)
	}
	return !errors.Is(err, os.ErrNotExist), nil
}

// Cleanup removes the specified file or directory and prints any errors
// to stderr. It's supposed to be used in defer statements to clean up
// temporary directories.
func Cleanup(path string) {
	if os.Getenv("SKIP_CLEANUP") != "" {
		return
	}

	err := os.RemoveAll(path)
	if err != nil {
		log.Warnf("%+v", errors.WithStack(err))
	}
}

// PrettifyPath prints a possibly shortened path for display purposes.
// If path is located under the current working directory, the relative path to
// it is returned, otherwise or in case of an error the path is returned
// unchanged.
func PrettifyPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, filepath.FromSlash("../")) {
		return path
	}
	return rel
}

// IsBelow returns true if and only if path lies below or is the path root.
// path and root must be either both absolute or both relative.
func IsBelow(path string, root string) (bool, error) {
	if filepath.IsAbs(path) != filepath.IsAbs(root) {
		return false, errors.Errorf("arguments to IsBelow must either both be relative or both be absolute, got: %q and %q", path, root)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		// Windows paths on separate drives can't be made relative to another.
		// Thus, instead of returning an error, IsBelow should just return
		// False if it fails to make the paths relative.
		// Note: filepath.Rel may also return an error if it would need to know
		// the current working directory, but that is not possible here since we
		// already enforce consistent path styles.
		return false, nil
	}
	return rel != ".." && !strings.HasPrefix(rel, filepath.FromSlash("../")), nil
}

// ForceSymlink creates newname as a symbolic link to oldname. If
// newname already exists, it will be overwritten.
func ForceSymlink(oldname, newname string) error {
	newnameTmp := newname + ".tmp"
	err := os.Remove(newnameTmp)
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	err = os.Symlink(oldname, newnameTmp)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.Rename(newnameTmp, newname)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ForceLongPathTempDir ensures that os.TempDir() creates temporary directories
// with long paths on Windows, resolving all "8.3" style short names. This is
// necessary because some external tools automatically resolve short paths to
// long paths, which can cause problems when trying to relativize paths.
//
// We prevent this issue by forcing long paths in os.TempDir() rather than
// normalizing paths whenever we relativize them for the following reasons:
//   - Go does not provide a function that normalizes paths without following
//     symlinks, so we would have to implement it ourselves.
//   - The temporary directories we create in cifuzz appear to be the only common
//     source of 8.3 paths and users are very unlikely to launch cifuzz from an
//     8.3 path.
func ForceLongPathTempDir() {
	if runtime.GOOS != "windows" {
		return
	}
	tempDirLongPath, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		log.Error(err, "Failed to get long path for temp dir: %v", err)
		return
	}
	// os.TempDir() calls GetTempPath on Windows, which first inspects
	// the TMP environment variable.
	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-gettemppatha
	err = os.Setenv("TMP", tempDirLongPath)
	if err != nil {
		log.Error(err, "Failed to set TMP to long path for temp dir: %v", err)
	}
}

// SearchFileBackwards searches for a file by going backwards/upwards
// from a given path
// if a path `/foo/bar` is given the order of search is
//  1. /foo/bar
//  2. /foo/
//  3. /
func SearchFileBackwards(start, filename string) (string, error) {
	currentDir := start
	for {
		filePath := filepath.Join(currentDir, filename)
		exists, err := Exists(filePath)
		if err != nil {
			return "", err
		}
		if exists {
			return filePath, nil
		}

		// if the root directory is reached stop the search
		if currentDir == filepath.Dir(currentDir) {
			break
		}

		// step one dir up
		currentDir = filepath.Dir(currentDir)
	}

	return "", os.ErrNotExist
}

// Runtime dependencies of fuzz tests that live under these paths will not be included in the artifact archive and have
// to be provided by the Docker image instead.
var systemLibraryPaths = map[string][]string{
	"linux": {
		"/lib",
		"/lib64",
		"/lib32",
		"/libx32",
		"/usr/lib",
	},
	"darwin": {
		"/lib",
		"/libexec",
		"/usr/lib",
		"/usr/libexec",
	},
}

var sharedLibraryRegex = regexp.MustCompile(`^.+\.((so)|(dylib))(\.\d\w*)*$`)

func IsSharedLibrary(path string) bool {
	return sharedLibraryRegex.MatchString(path)
}

func IsSystemLibrary(library string) bool {
	for _, systemLibraryPath := range systemLibraryPaths[runtime.GOOS] {
		isBelowLibPath, err := IsBelow(library, systemLibraryPath)
		if err != nil {
			log.Errorf(err, "error while checking system library %s: %v", library, err)
			return false
		}
		if isBelowLibPath {
			return true
		}
	}

	return false
}
