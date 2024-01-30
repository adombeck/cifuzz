package dependencies

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/pkg/mocks"
)

type versionTest struct {
	Want   *semver.Version
	Regex  *regexp.Regexp
	Output string
}

var tests = []versionTest{
	// ---cmake
	{
		Want:  semver.MustParse("3.24.1"),
		Regex: cmakeRegex,
		Output: `cmake version 3.24.1

CMake suite maintained and supported by Kitware (kitware.com/cmake).`,
	},
	{
		Want:   semver.MustParse("3.21.0"),
		Regex:  cmakeRegex,
		Output: `cmake version 3.21.0`,
	},
	// ---clang
	{
		Want:  semver.MustParse("14.0.6"),
		Regex: clangRegex,
		Output: `clang version 14.0.6
Target: x86_64-pc-linux-gnu
Thread model: posix
InstalledDir: /usr/sbin`,
	},
	{
		Want:  semver.MustParse("14.0.6"),
		Regex: clangRegex,
		Output: `Debian clang version 14.0.6-2
Target: x86_64-pc-linux-gnu
Thread model: posix
InstalledDir: /usr/bin`,
	},
	{
		Want:   semver.MustParse("14.0.0"),
		Regex:  clangRegex,
		Output: `foobar clang version 14.0-special`,
	},
	// ---llvm-symbolizer
	{
		Want:  semver.MustParse("14.0.6"),
		Regex: llvmRegex,
		Output: `llvm-symbolizer
LLVM (http://llvm.org/):
  LLVM version 14.0.6
  Optimized build.
  Default target: x86_64-pc-linux-gnupa
  Host CPU: znver3`,
	},
	{
		Want:  semver.MustParse("14.0.6"),
		Regex: llvmRegex,
		Output: `llvm-symbolizer
Debian LLVM version 14.0.6

  Optimized build.
  Default target: x86_64-pc-linux-gnu
  Host CPU: skylake`,
	},
	{
		Want:  semver.MustParse("1.8.0"),
		Regex: javaRegex,
		Output: `openjdk version "1.8.0_265"
OpenJDK Runtime Environment (AdoptOpenJDK)(build 1.8.0_265-b01)
OpenJDK 64-Bit Server VM (AdoptOpenJDK)(build 25.265-b01, mixed mode)`,
	},
	{
		Want:  semver.MustParse("18.0.0"),
		Regex: javaRegex,
		Output: `openjdk version "18" 2022-03-22
OpenJDK Runtime Environment (build 18+36-2087)
OpenJDK 64-Bit Server VM (build 18+36-2087, mixed mode, sharing)`,
	},
	{
		Want:  semver.MustParse("18.0.0"),
		Regex: javaRegex,
		Output: `openjdk version "18.0.0.1" 2022-03-22
OpenJDK Runtime Environment (build 18+36-2087)
OpenJDK 64-Bit Server VM (build 18+36-2087, mixed mode, sharing)`,
	},
	{
		Want:   semver.MustParse("16.16.0"),
		Regex:  nodeRegex,
		Output: `v16.16.0`,
	},
	{
		Want:   semver.MustParse("0.19.0"),
		Regex:  jazzerRegex,
		Output: `.m2/repository/com/code-intelligence/jazzer/0.19.0/jazzer-0.19.0.jar`,
	},
	{
		Want:   semver.MustParse("5.9.2"),
		Regex:  junitRegex,
		Output: `.m2/repository/org/junit/jupiter/junit-jupiter-engine/5.9.2/junit-jupiter-engine-5.9.2.jar`,
	},
}

func TestVersionParsing(t *testing.T) {
	for i, test := range tests {
		key := Key(fmt.Sprintf("version-test-%d", i))
		version, err := extractVersion(test.Output, test.Regex, key)
		require.NoError(t, err)
		require.True(t, version.Equal(test.Want),
			"%s: expected version %s, got %s", key, test.Want.String(), version.String())
	}
}

func TestClangVersion_AllEnv(t *testing.T) {
	ccVersion := semver.MustParse("10.0.0")
	cxxVersion := semver.MustParse("11.0.0")
	mockCheck := func(path string, key Key) (*semver.Version, error) {
		switch path {
		case "CC/clang":
			return ccVersion, nil
		case "CXX/clang":
			return cxxVersion, nil
		}
		return nil, nil
	}

	t.Setenv("CC", "CC/clang")
	t.Setenv("CXX", "CXX/clang")

	keys := []Key{Clang}
	dep := getDeps(keys)[Clang]

	version, err := clangVersion(dep, mockCheck)
	require.NoError(t, err)

	// we expect the cc version as it is lower than cxx
	assert.Equal(t, ccVersion, version)
}

func TestClangVersion_CCMissing(t *testing.T) {
	cxxVersion := semver.MustParse("10.0.0")
	pathVersion := semver.MustParse("1.0.0")
	mockCheck := func(path string, key Key) (*semver.Version, error) {
		switch path {
		case "CC/clang++":
			return cxxVersion, nil
		case "path/clang":
			return pathVersion, nil
		}
		return nil, nil
	}

	finderMock := &mocks.RunfilesFinderMock{}
	finderMock.On("ClangPath").Return("path/clang", nil)

	t.Setenv("CC", "")
	t.Setenv("CXX", "CC/clang++")

	keys := []Key{Clang}
	dep := getDeps(keys)[Clang]
	dep.finder = finderMock

	version, err := clangVersion(dep, mockCheck)
	require.NoError(t, err)

	assert.Equal(t, cxxVersion, version)
}

func TestClangVersion_CCFilename(t *testing.T) {
	filename := "my-clang-13"
	version := semver.MustParse("13.0.0")

	mockCheck := func(path string, key Key) (*semver.Version, error) {
		return version, nil
	}

	finderMock := &mocks.RunfilesFinderMock{}
	finderMock.On("ClangPath").Return("path/clang", nil)

	t.Setenv("CC", filename)
	t.Setenv("CXX", "g++")

	keys := []Key{Clang}
	dep := getDeps(keys)[Clang]
	dep.finder = finderMock

	versionFound, err := clangVersion(dep, mockCheck)
	require.NoError(t, err)
	assert.Equal(t, version, versionFound)
}

func TestClangVersion_CXXFilename(t *testing.T) {
	filename := "my-clang++-13"
	version := semver.MustParse("13.0.0")

	mockCheck := func(path string, key Key) (*semver.Version, error) {
		return version, nil
	}

	finderMock := &mocks.RunfilesFinderMock{}
	finderMock.On("ClangPath").Return("path/clang", nil)

	t.Setenv("CC", "gcc")
	t.Setenv("CXX", filename)

	keys := []Key{Clang}
	dep := getDeps(keys)[Clang]
	dep.finder = finderMock

	versionFound, err := clangVersion(dep, mockCheck)
	require.NoError(t, err)
	assert.Equal(t, version, versionFound)
}
