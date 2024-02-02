package dependencies

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/gradle"
	"code-intelligence.com/cifuzz/pkg/log"
)

type Dependencies map[Key]*Dependency

// List of all known dependencies
var deps = Dependencies{
	Bazel: {
		Key:        Bazel,
		MinVersion: getMinVersionBazel(),
		GetVersion: bazelVersion,
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.BazelPath)
		},
	},
	Clang: {
		Key:        Clang,
		MinVersion: *semver.MustParse("11.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			return clangVersion(dep, clangCheck)
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			var clang string
			cc := os.Getenv("CC")
			if cc != "" && strings.Contains(path.Base(cc), "clang") {
				clang = cc
			}

			if clang == "" {
				cxx := os.Getenv("CXX")
				if cxx != "" && strings.Contains(path.Base(cxx), "clang") {
					clang = cxx
				}
			}

			if clang != "" {
				_, err := exec.LookPath(clang)
				if err == nil {
					return true
				}
			}

			return dep.checkFinder(dep.finder.ClangPath)
		},
	},
	CMake: {
		Key:        CMake,
		MinVersion: *semver.MustParse("3.16.0"),
		GetVersion: cmakeVersion,
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.CMakePath)
		},
	},
	LLVMCov: {
		Key:        LLVMCov,
		MinVersion: *semver.MustParse("11.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			path, err := dep.finder.LLVMCovPath()
			if err != nil {
				return nil, err
			}
			version, err := llvmVersion(path, dep)
			if err != nil {
				return nil, err
			}
			log.Debugf("Found llvm-cov version %s in: %s", version, path)
			return version, nil
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.LLVMCovPath)
		},
	},
	LLVMProfData: {
		Key: LLVMProfData,
		// llvm-profdata provides no version information
		MinVersion: *semver.MustParse("0.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			return semver.NewVersion("0.0.0")
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			path, err := dep.finder.LLVMProfDataPath()
			if err != nil {
				return false
			}
			log.Debugf("Found llvm-profdata in: %s", path)
			return true
		},
	},
	LLVMSymbolizer: {
		Key:        LLVMSymbolizer,
		MinVersion: *semver.MustParse("11.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			path, err := dep.finder.LLVMSymbolizerPath()
			if err != nil {
				return nil, err
			}
			version, err := llvmVersion(path, dep)
			if err != nil {
				return nil, err
			}
			log.Debugf("Found llvm-symbolizer version %s in: %s", version, path)
			return version, nil
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.LLVMSymbolizerPath)
		},
	},
	GenHTML: {
		Key:        GenHTML,
		MinVersion: *semver.MustParse("0.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			path, err := dep.finder.GenHTMLPath()
			if err != nil {
				return nil, err
			}
			version, err := genHTMLVersion(path, dep)
			if err != nil {
				return nil, err
			}
			log.Debugf("Found genHTML version %s in PATH: %s", version, path)
			return version, nil
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.GenHTMLPath)
		},
	},
	Perl: {
		Key:        Perl,
		MinVersion: *semver.MustParse("0.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			return semver.NewVersion("0.0.0")
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.PerlPath)
		},
	},
	Java: {
		Key:        Java,
		MinVersion: *semver.MustParse("1.8.0"),
		GetVersion: javaVersion,
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.JavaHomePath)
		},
	},
	Maven: {
		Key:        Maven,
		MinVersion: *semver.MustParse("0.0.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			return semver.NewVersion("0.0.0")
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.MavenPath)
		},
	},
	Gradle: {
		Key:        Gradle,
		MinVersion: *semver.MustParse("0.0.0"),
		GetVersion: gradleVersion,
		Installed: func(dep *Dependency, projectDir string) bool {
			if projectDir != "" {
				// Using the gradlew in the project dir is the preferred way
				wrapper, err := gradle.FindGradleWrapper(projectDir)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					log.Error(errors.Wrap(err, "Error while checking for existing 'gradlew' in project dir. Gradle will be checked instead"))
					return dep.checkFinder(dep.finder.GradlePath)
				}
				if wrapper != "" {
					return true
				}
			}

			return dep.checkFinder(dep.finder.GradlePath)
		},
	},
	Node: {
		Key:        Node,
		MinVersion: *semver.MustParse("16.0"),
		GetVersion: nodeVersion,
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.NodePath)
		},
	},
	VisualStudio: {
		Key:        VisualStudio,
		MinVersion: *semver.MustParse("17.0"),
		GetVersion: func(dep *Dependency, projectDir string) (*semver.Version, error) {
			return visualStudioVersion()
		},
		Installed: func(dep *Dependency, projectDir string) bool {
			return dep.checkFinder(dep.finder.VisualStudioPath)
		},
	},
}

func getMinVersionBazel() semver.Version {
	if runtime.GOOS == "darwin" {
		return *semver.MustParse("6.0.0")
	}

	return *semver.MustParse("5.3.2")
}

// CIFuzzBazelCommit is the commit of the
// https://github.com/adombeck/cifuzz-bazel
// repository that is required by this version of cifuzz.
//
// Keep in sync with examples/bazel/WORKSPACE.
const CIFuzzBazelCommit = "b013aa0f90fe8ac60adfc6d9640a9cfa451dda9e"

const RulesFuzzingSHA256 = "4beab98d88e4bf2d04da0412d413a1364f95e5eb88963e15e603bee1410fcedf"

var RulesFuzzingHTTPArchiveRule = fmt.Sprintf(`http_archive(
        name = "rules_fuzzing",
        sha256 = "%s",
        strip_prefix = "rules_fuzzing-ca617e846d0f92e00a903903b0554ea9142e1132",
        urls = ["https://github.com/adombeck/rules_fuzzing/archive/ca617e846d0f92e00a903903b0554ea9142e1132.tar.gz"],
    )`, RulesFuzzingSHA256)
