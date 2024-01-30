//go:build !freebsd && !linux && !windows

package ldd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/util/fileutil"
)

// For operating systems other than Linux and FreeBSD, we resort to scanning the working directory
// for shared object. The ldd package from the u-root project only implements this functionality
// correctly for these two systems.

// TODO implement dependency resolution for other systems when needed. For darwin we can use
// the standard debug/macho package and recursively list the dependencies from the main executable
// and its transitive dependencies. We also need to resolve the variables @executable_path,
// @loader_path and @rpath from install IDs of the dynamic libraries.
func NonSystemSharedLibraries(executable string) ([]string, error) {
	var sharedObjects []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if info.IsDir() {
			return nil
		}
		// Ignore shared objects in .dSYM directories, to avoid llvm-cov
		// failing with:
		//
		//    Failed to load coverage: Unsupported coverage format version
		//
		if strings.Contains(path, "dSYM") {
			return nil
		}
		if fileutil.IsSharedLibrary(info.Name()) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return errors.WithStack(err)
			}
			sharedObjects = append(sharedObjects, absPath)
		}
		return nil
	})
	return sharedObjects, errors.WithMessage(err, "Failed to gather non system shared libraries")
}
