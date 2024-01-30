package dependencies

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/java/gradle"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
)

/*
Note: we made the "patch" part of the semver (when parsing the output with regex) optional
be more lenient when a command returns something like 1.2 instead of 1.2.0
*/
var (
	clangRegex  = regexp.MustCompile(`(?m)clang version (?P<version>\d+\.\d+(\.\d+)?)`)
	cmakeRegex  = regexp.MustCompile(`(?m)cmake version (?P<version>\d+\.\d+(\.\d+)?)`)
	llvmRegex   = regexp.MustCompile(`(?m)LLVM version (?P<version>\d+\.\d+(\.\d+)?)`)
	javaRegex   = regexp.MustCompile(`(?m)version "(?P<version>\d+(\.\d+\.\d+)*)([_\.]\d+)?"`)
	gradleRegex = regexp.MustCompile(`(?m)Gradle (?P<version>\d+(\.\d+\.\d+)?)`)
	nodeRegex   = regexp.MustCompile(`(?m)(?P<version>\d+(\.\d+\.\d+)?)`)

	bazelRegex   = regexp.MustCompile(`(?m)bazel (?P<version>\d+(\.\d+\.\d+)?)`)
	genHTMLRegex = regexp.MustCompile(`.*LCOV version (?P<version>\d+\.\d+(\.\d+)?)`)

	jazzerRegex = regexp.MustCompile(`jazzer-(?P<version>\d+\.\d+\.\d+).jar`)
	junitRegex  = regexp.MustCompile(`junit-jupiter-engine-(?P<version>\d+\.\d+\.\d+).jar`)
)

type execCheck func(string, Key) (*semver.Version, error)

func bazelVersion(dep *Dependency, projectDir string) (*semver.Version, error) {
	path, err := exec.LookPath("bazel")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	version, err := getVersionFromCommand(path, []string{"--version"}, bazelRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found Bazel version %s in PATH: %s", version, path)
	return version, nil
}

// small helper to reuse clang version check
func clangCheck(path string, key Key) (*semver.Version, error) {
	version, err := getVersionFromCommand(path, []string{"--version"}, clangRegex, key)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// returns the currently installed clang version
func clangVersion(dep *Dependency, clangCheck execCheck) (*semver.Version, error) {
	var clangVersion *semver.Version
	env := os.Environ()

	// First we check if the environment variables CC and CXX are set
	// and contain a valid version number. If both contain a valid version
	// number, we return the smallest one. If both are not set, we also check the
	// clang available in the path
	cc := envutil.GetEnvWithPathSubstring(env, "CC", "clang")
	if cc != "" {
		ccVersion, err := clangCheck(cc, dep.Key)
		if err != nil {
			return nil, err
		}
		log.Debugf("Found clang version %s in CC: %s", ccVersion.String(), cc)
		clangVersion = ccVersion
	} else {
		clang, err := exec.LookPath("clang")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		log.Warn("No clang found in CC, now using ", clang)
	}

	cxx := envutil.GetEnvWithPathSubstring(env, "CXX", "clang++")
	if cxx != "" {
		cxxVersion, err := clangCheck(cxx, dep.Key)
		if err != nil {
			return nil, err
		}
		log.Debugf("Found clang++ version %s in CXX: %s", cxxVersion.String(), cxx)
		if clangVersion == nil || clangVersion.GreaterThan(cxxVersion) {
			clangVersion = cxxVersion
		}
		if !clangVersion.Equal(cxxVersion) {
			log.Warn(`clang and clang++ versions are different.
Other llvm tools like llvm-cov are selected based on the smaller version.`)
		}
	} else {
		clangPP, err := exec.LookPath("clang++")
		if err != nil {
			return nil, errors.WithStack(err)
		}
		log.Warn("No clang++ found in CC, now using ", clangPP)
	}

	if clangVersion == nil {
		path, err := dep.finder.ClangPath()
		if err != nil {
			return nil, err
		}
		pathVersion, err := clangCheck(path, dep.Key)
		if err != nil {
			return nil, err
		}
		log.Debugf("Found clang version %s in PATH: %s", pathVersion.String(), path)
		clangVersion = pathVersion
	}

	return clangVersion, nil

}

// helper for parsing the --version output for different llvm tools,
// for example llvm-cov, llvm-symbolizer
func llvmVersion(path string, dep *Dependency) (*semver.Version, error) {
	version, err := getVersionFromCommand(path, []string{"--version"}, llvmRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func genHTMLVersion(path string, dep *Dependency) (*semver.Version, error) {
	version, err := getVersionFromCommand(path, []string{"--version"}, genHTMLRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func cmakeVersion(dep *Dependency, projectDir string) (*semver.Version, error) {
	path, err := exec.LookPath("cmake")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	version, err := getVersionFromCommand(path, []string{"--version"}, cmakeRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found CMake version %s in PATH: %s", version, path)
	return version, nil
}

func javaVersion(dep *Dependency, projectDir string) (*semver.Version, error) {
	javaBin, err := runfiles.Finder.JavaPath()
	if err != nil {
		return nil, err
	}

	version, err := getVersionFromCommand(javaBin, []string{"-version"}, javaRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found Java version %s in PATH: %s", version, javaBin)
	return version, nil
}

func gradleVersion(dep *Dependency, projectDir string) (*semver.Version, error) {
	path, err := gradle.GetGradleCommand(projectDir)
	if err != nil {
		return nil, err
	}

	version, err := getVersionFromCommand(path, []string{"-version"}, gradleRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found Gradle version %s: %s", version, path)
	return version, nil
}

func JazzerVersion(classPath string) (*semver.Version, error) {
	return extractVersion(classPath, jazzerRegex, "jazzer")
}

func JUnitVersion(classPath string) (*semver.Version, error) {
	return extractVersion(classPath, junitRegex, "junit-jupiter-engine")
}

func JavaVersion(javaBin string) (*semver.Version, error) {
	return getVersionFromCommand(javaBin, []string{"-version"}, javaRegex, Java)
}

func JazzerJSVersion() (*semver.Version, error) {
	cmd := exec.Command("npx", []string{"jazzer", "--version"}...)
	output, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	return extractVersion(string(output), nodeRegex, "jazzer.js")
}

func JestVersion() (*semver.Version, error) {
	cmd := exec.Command("npx", []string{"jest", "--version"}...)
	output, err := cmd.Output()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	return extractVersion(string(output), nodeRegex, "jest")
}

func nodeVersion(dep *Dependency, projectDir string) (*semver.Version, error) {
	path, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	version, err := getVersionFromCommand(path, []string{"--version"}, nodeRegex, dep.Key)
	if err != nil {
		return nil, err
	}
	log.Debugf("Found Node version %s in PATH: %s", version, path)
	return version, nil
}

func visualStudioVersion() (*semver.Version, error) {
	var vsVersion *semver.Version
	versionFromEnv := os.Getenv("VisualStudioVersion")

	version, err := semver.NewVersion(versionFromEnv)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	vsVersion = version
	return vsVersion, nil
}

// takes a command + args and parses the output for a semver
func getVersionFromCommand(cmdPath string, args []string, re *regexp.Regexp, key Key) (*semver.Version, error) {
	output := bytes.Buffer{}
	cmd := exec.Command(cmdPath, args...)
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return extractVersion(output.String(), re, key)
}

func extractVersion(output string, re *regexp.Regexp, key Key) (*semver.Version, error) {
	result := re.FindStringSubmatch(output)
	if len(result) <= 1 {
		return nil, fmt.Errorf("no matching version string for %s", key)
	}

	version, err := semver.NewVersion(result[1])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return version, nil
}
