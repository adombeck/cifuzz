package resolve

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/java/gradle"
	"code-intelligence.com/cifuzz/internal/build/java/maven"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/regexutil"
)

// TODO: use file info of cmake instead of this regex
var cmakeFuzzTestFileNamePattern = regexp.MustCompile(`add_fuzz_test\((?P<fuzzTest>[a-zA-Z0-9_.+=,@~-]+)\s(?P<file>[a-zA-Z0-9/\_.+=,@~-]+)\)`)

// resolve determines the corresponding fuzz test name to a given source file.
// The path has to be relative to the project directory.
func resolve(path, buildSystem, projectDir string) (string, error) {
	switch buildSystem {
	case config.BuildSystemCMake:
		cmakeLists, err := findAllCMakeLists(projectDir)
		if err != nil {
			return "", err
		}

		for _, list := range cmakeLists {
			var bs []byte
			bs, err = os.ReadFile(filepath.Join(projectDir, list))
			if err != nil {
				return "", errors.WithStack(err)
			}

			if !strings.Contains(string(bs), "add_fuzz_test") {
				continue
			}

			matches, _ := regexutil.FindAllNamedGroupsMatches(cmakeFuzzTestFileNamePattern, string(bs))
			for _, match := range matches {
				if (filepath.IsAbs(path) && filepath.Join(projectDir, filepath.Dir(list), match["file"]) == path) ||
					filepath.Join(filepath.Dir(list), match["file"]) == path {
					return match["fuzzTest"], nil
				}
			}
		}
		return "", errors.New("no fuzz test found")

	case config.BuildSystemBazel:
		var err error
		if filepath.IsAbs(path) {
			path, err = filepath.Rel(projectDir, path)
			if err != nil {
				return "", errors.WithStack(err)
			}
		}

		if runtime.GOOS == "windows" {
			// bazel doesn't allow backslashes in its query
			// but it would be unusual for windows users to
			// use slashes when writing a path so we allow
			// backslashes and replace them internally
			path = strings.ReplaceAll(path, "\\", "/")
		}
		arg := fmt.Sprintf(`attr(generator_function, cc_fuzz_test, same_pkg_direct_rdeps(%q))`, path)
		cmd := exec.Command("bazel", "query", arg)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			// if a bazel query fails it is because no target could be found but it would
			// only return "exit status 7" as error which is no useful information for
			// the user, so instead we return the custom error
			return "", errors.New("no fuzz test found")
		}

		fuzzTest := strings.TrimSpace(string(out))
		fuzzTest = strings.TrimSuffix(fuzzTest, "_raw_")

		return fuzzTest, nil

	case config.BuildSystemMaven, config.BuildSystemGradle:
		var testDirs []string
		var err error
		if buildSystem == config.BuildSystemMaven {
			testDir, err := maven.GetTestDir(projectDir)
			if err != nil {
				return "", err
			}
			testDirs = append(testDirs, testDir)
		} else if buildSystem == config.BuildSystemGradle {
			testDirs, err = gradle.GetTestSourceSets(projectDir)
			if err != nil {
				return "", err
			}
		}

		var fuzzTest string
		found := false
		for _, testDir := range testDirs {
			// Handle case that gradle or maven command return default values that don't actually exist
			exist, err := fileutil.Exists(testDir)
			if err != nil {
				return "", errors.WithMessagef(err, "Failed to access test directory %s", testDir)
			}
			if !exist {
				continue
			}

			matches, err := zglob.Glob(filepath.Join(testDir, "**", "*.{java,kt}"))
			if err != nil {
				return "", errors.WithStack(err)
			}

			var pathToFile string
			for _, match := range matches {
				if (filepath.IsAbs(path) && match == path) ||
					match == filepath.Join(projectDir, path) {
					pathToFile = match
					found = true
				}

				if !found && runtime.GOOS == "windows" {
					// Try out different slashes under windows to support both formats for user convenience
					// (since zglob.Glob() returns paths with slashes instead of backslashes on windows)
					match = strings.ReplaceAll(match, "/", "\\")
					if (filepath.IsAbs(path) && match == path) ||
						match == filepath.Join(projectDir, path) {
						pathToFile = match
						found = true
					}
				}
			}
			if !found {
				continue
			}

			fuzzTest, err = cmdutils.ConstructJVMFuzzTestIdentifier(pathToFile, testDir)
			if err != nil {
				return "", err
			}
			break
		}
		if !found {
			return "", errors.New("no fuzz test found")
		}
		return fuzzTest, nil

	case config.BuildSystemNodeJS:
		testFile := filepath.Base(path)
		fuzzTest := strings.TrimSuffix(testFile, ".fuzz"+filepath.Ext(testFile))

		return fuzzTest, nil

	default:
		return "", errors.New("The flag '--resolve' only supports the following build systems: CMake, Bazel, Maven, Gradle.")
	}
}

func findAllCMakeLists(projectDir string) ([]string, error) {
	var cmakeLists []string

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		path, err = filepath.Rel(projectDir, path)
		if err != nil {
			return errors.WithStack(err)
		}

		baseName := filepath.Base(path)
		if baseName == "CMakeLists.txt" {
			cmakeLists = append(cmakeLists, path)
		}

		return nil
	})

	return cmakeLists, errors.WithStack(err)
}

func FuzzTestArguments(resolveSourceFile bool, args []string, buildSystem, projectDir string) ([]string, error) {
	if resolveSourceFile {
		var fuzzTests []string
		for _, arg := range args {
			fuzzTest, err := resolve(arg, buildSystem, projectDir)
			if err != nil {
				return nil, errors.WithMessagef(err, "Failed to resolve source file %s", arg)
			}
			fuzzTests = append(fuzzTests, fuzzTest)
		}
		return fuzzTests, nil
	}

	return args, nil
}
