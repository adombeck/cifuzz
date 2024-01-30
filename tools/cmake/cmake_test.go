package cmake

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const cifuzzCmakeBuildType = "RelWithDebInfo"

var baseTempDir string

func TestMain(m *testing.M) {
	var err error

	// The CMake integration is installed globally once and used by all tests.
	baseTempDir, err = os.MkdirTemp("", "cmake-test-")
	if err != nil {
		log.Fatalf("Failed to create temp dir for tests: %+v", err)
	}

	installDir := filepath.Join(baseTempDir, "install-dir")
	opts := builderPkg.Options{
		Version:   "dev",
		TargetDir: installDir,
		Coverage:  true,
	}
	builder, err := builderPkg.NewCIFuzzBuilder(opts)
	if err != nil {
		builder.Cleanup()
		log.Fatalf("Failed to install CMake integration: %+v", err)
	}
	err = builder.BuildDumper()
	if err != nil {
		builder.Cleanup()
		log.Fatalf("Failed to install CMake integration: %+v", err)
	}
	err = builder.CopyFiles()
	if err != nil {
		builder.Cleanup()
		log.Fatalf("Failed to install CMake integration: %+v", err)
	}

	// Include the CMake package by setting the CMAKE_PREFIX_PATH.
	cmakePrefixPathEnv := os.Getenv("CMAKE_PREFIX_PATH")
	defer func() {
		err = os.Setenv("CMAKE_PREFIX_PATH", cmakePrefixPathEnv)
		if err != nil {
			log.Fatalf("Failed to restore CMAKE_PREFIX_PATH: %+v", err)
		}
	}()
	err = os.Setenv("CMAKE_PREFIX_PATH", filepath.Join(installDir, "share", "cmake"))
	if err != nil {
		builder.Cleanup()
		log.Panicf("Failed to install CMake integration: %+v", err)
	}

	m.Run()

	builder.Cleanup()
}

func TestIntegration_Ctest_DefaultSettings(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	// Simulate a build without any special flags. This is closest to what users get when they run fuzz tests like their
	// existing (unit) tests - without cifuzz run or any CIFUZZ_* CMake variables set.
	buildDir := build(t, "", nil)
	// The default configuration without any other settings is Debug.
	runAndAssertTests(t, buildDir, "Debug", map[string]bool{
		// Without sanitizers, the seed corpus entries do not crash this target.
		"parser_fuzz_test_regression_test": true,
		// The target returns a non-zero value on every input and the replayer always runs on the empty input.
		"no_seed_corpus_fuzz_test_regression_test": false,
		// Never crashes.
		"c_fuzz_test_regression_test": true,
	})
}

func TestIntegration_Ctest_WithAddressSanitizer(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	buildDir := build(t, cifuzzCmakeBuildType, map[string]string{
		"CIFUZZ_SANITIZERS": "address",
		"CIFUZZ_TESTING":    "ON",
	})
	runAndAssertTests(t, buildDir, cifuzzCmakeBuildType, map[string]bool{
		// Crashes on the `asan_crash` input.
		"parser_fuzz_test_regression_test": false,
		// The target returns a non-zero value on every input and the replayer always runs on the empty input.
		"no_seed_corpus_fuzz_test_regression_test": false,
		// Never crashes.
		"c_fuzz_test_regression_test": true,
	})
}

func TestIntegration_Ctest_WithUndefinedBehaviorSanitizer(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	buildDir := build(t, cifuzzCmakeBuildType, map[string]string{
		"CIFUZZ_SANITIZERS": "undefined",
		"CIFUZZ_TESTING":    "ON",
	})
	runAndAssertTests(t, buildDir, cifuzzCmakeBuildType, map[string]bool{
		// Crashes on the `ubsan_crash` input.
		"parser_fuzz_test_regression_test": false,
		// The target returns a non-zero value on every input and the replayer always runs on the empty input.
		"no_seed_corpus_fuzz_test_regression_test": false,
		// Never crashes.
		"c_fuzz_test_regression_test": true,
	})
}

func TestIntegration_Build_WithMultipleSanitizers(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	build(t, cifuzzCmakeBuildType, map[string]string{
		"CIFUZZ_SANITIZERS": "address;undefined",
		"CIFUZZ_TESTING":    "ON",
	})
}

func TestIntegration_Build_LegacyFuzzTests(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	buildDir := build(t, cifuzzCmakeBuildType, map[string]string{"CIFUZZ_USE_DEPRECATED_MACROS": "ON"})
	runAndAssertTests(t, buildDir, cifuzzCmakeBuildType, map[string]bool{"legacy_fuzz_test_regression_test": true})
}

func TestIntegration_CIFuzzInfoIsCreated(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	buildDir, err := os.MkdirTemp(baseTempDir, "build")
	require.NoError(t, err)

	// Only configure, don't build.
	runInDir(t, buildDir, "cmake", testDataDir(t))

	// For multi-configuration build tools such as MSBuild, the build directory contains subdirectories for each
	// configuration. Since we didn't specify any explicitly, the default is "Debug". With make and ninja, the value of
	// the $<CONFIG> generator expression used in the CMake integration is ".".
	var configDir string
	var binaryName string
	if runtime.GOOS == "windows" {
		configDir = "Debug"
		binaryName = "parser_fuzz_test.exe"
	} else {
		configDir = "."
		binaryName = "parser_fuzz_test"
	}

	// The CMake integration should create a file containing the binary path for every target at a fixed location.
	parserFuzzTestInfo := filepath.Join(buildDir, configDir, ".cifuzz", "fuzz_tests", "parser_fuzz_test", "executable")
	content, err := os.ReadFile(parserFuzzTestInfo)
	require.NoError(t, err)
	// Canonicalize the paths before comparing since they may use different symlinks (e.g., on macOS /private/var and
	// /var are symlinked aliases). Since Go doesn't offer a way to canonicalize paths to non-existent files, touch both
	// paths (the fuzz test binaries haven't been built yet since we only ran the configure step).
	expectedFuzzTestPath := filepath.Join(buildDir, "src", "parser", configDir, binaryName)
	err = os.MkdirAll(filepath.Dir(expectedFuzzTestPath), 0755)
	require.NoError(t, err)
	err = fileutil.Touch(expectedFuzzTestPath)
	require.NoError(t, err)
	expectedFuzzTestPath, err = filepath.EvalSymlinks(expectedFuzzTestPath)
	require.NoError(t, err)
	actualFuzzTestPath := string(content)
	err = os.MkdirAll(filepath.Dir(actualFuzzTestPath), 0755)
	require.NoError(t, err)
	err = fileutil.Touch(actualFuzzTestPath)
	require.NoError(t, err)
	actualFuzzTestPath, err = filepath.EvalSymlinks(actualFuzzTestPath)
	require.NoError(t, err)
	assert.Equal(t, expectedFuzzTestPath, actualFuzzTestPath)

	// The integration should also create a file containing the path of the seed corpus.
	seedCorpusInfo := filepath.Join(buildDir, configDir, ".cifuzz", "fuzz_tests", "parser_fuzz_test", "seed_corpus")
	content, err = os.ReadFile(seedCorpusInfo)
	require.NoError(t, err)
	seedCorpusPath := string(content)
	require.DirExists(t, seedCorpusPath)
	require.FileExists(t, filepath.Join(seedCorpusPath, "asan_crash"))
	require.FileExists(t, filepath.Join(seedCorpusPath, "ubsan_crash"))
}

func TestIntegration_RuntimeDepsInfo(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if runtime.GOOS != "linux" {
		// TODO(fmeum): The CMake GET_RUNTIME_DEPENDENCIES feature doesn't seem to handle macOS' @rpath feature well.
		//              When executed on macOS, this test fails with:
		//              UNRESOLVED @rpath/libc++.1.dylib
		//              Since @rpath is resolved based on information embedded into the executable loading the shared
		//              library with this directive, it is possible that CMake fails to account for these paths when
		//              parsing the dependencies of the library by itself.
		//              It also fails on Windows with:
		//              UNRESOLVED api-ms-win-appmodel-runtime-internal-l1-1-2.dll
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	buildDir := build(t, cifuzzCmakeBuildType, nil)

	assert.ElementsMatch(t, []string{
		// Direct dependency
		"src/parser/libparser.so",
		// Transitive dependency
		"src/utils/libhelper.so",
	}, extractRuntimeDeps(t, buildDir, "parser_fuzz_test"))
	assert.Empty(t, extractRuntimeDeps(t, buildDir, "c_fuzz_test"))
}

const fakeCIFuzzSource = `
#include <stdio.h>

int main(int argc, char **argv) {
  for (int i = 1; i < argc; i++) {
    printf("%s\n", argv[i]);
  }
  return 0;
}
`

func TestIntegration_FuzzTestBinaryLaunchesCIFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	testutil.RegisterTestDeps("testdata", "modules")

	cmakeVariables := map[string]string{
		"CIFUZZ_ENGINE":     "libfuzzer",
		"CIFUZZ_SANITIZERS": "address",
		// Enable CIFUZZ_TESTING, which is needed in order to set
		// required compile and link options
		"CIFUZZ_TESTING": "ON",
	}
	if runtime.GOOS != "windows" {
		cmakeVariables["CMAKE_C_COMPILER"] = "clang"
		cmakeVariables["CMAKE_CXX_COMPILER"] = "clang++"
	}
	buildDir := build(t, cifuzzCmakeBuildType, cmakeVariables, "--target", "c_fuzz_test")

	// Using c_fuzz_test here as it doesn't have any shared library dependencies - those are not supported with fuzzing
	// instrumentation on Windows.
	cFuzzTestInfo := filepath.Join(buildDir, cifuzzCmakeBuildType, ".cifuzz", "fuzz_tests", "c_fuzz_test", "executable")
	fuzzTestPath, err := os.ReadFile(cFuzzTestInfo)
	require.NoError(t, err)

	// Verify that running the fuzz test directly executes cifuzz from the path by adding a fake cifuzz to PATH first in
	// search order.
	fakeCIFuzzDir, err := os.MkdirTemp(baseTempDir, "")
	require.NoError(t, err)
	fakeCIFuzzSrc := filepath.Join(fakeCIFuzzDir, "cifuzz.c")
	err = os.WriteFile(fakeCIFuzzSrc, []byte(fakeCIFuzzSource), 0o644)
	require.NoError(t, err)
	var fakeCIFuzz string
	var fakeCIFuzzCompileCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		fakeCIFuzz = filepath.Join(fakeCIFuzzDir, "cifuzz.exe")
		fakeCIFuzzCompileCmd = exec.Command("clang-cl", fakeCIFuzzSrc, "/Fe"+fakeCIFuzz)
	} else {
		fakeCIFuzz = filepath.Join(fakeCIFuzzDir, "cifuzz")
		fakeCIFuzzCompileCmd = exec.Command("clang", fakeCIFuzzSrc, "-o", fakeCIFuzz)
	}
	fakeCIFuzzCompileCmd.Stdout = os.Stdout
	fakeCIFuzzCompileCmd.Stderr = os.Stderr
	err = fakeCIFuzzCompileCmd.Run()
	require.NoError(t, err)

	cmd := exec.Command(string(fuzzTestPath))
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s%c%s", fakeCIFuzzDir, os.PathListSeparator, os.Getenv("PATH")))
	out, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "run\nc_fuzz_test\n", strings.ReplaceAll(string(out), "\r\n", "\n"))
}

func build(t *testing.T, buildType string, cacheVariables map[string]string, additionalBuildArgs ...string) string {
	buildDir, err := os.MkdirTemp(baseTempDir, "build")
	require.NoError(t, err)

	var cacheArgs []string
	for key, value := range cacheVariables {
		cacheArgs = append(cacheArgs, "-D", fmt.Sprintf("%s=%s", key, value))
	}
	cacheArgs = append(cacheArgs, "-DCMAKE_VERBOSE_MAKEFILE:BOOL=ON")
	if buildType != "" {
		cacheArgs = append(cacheArgs, "-D", fmt.Sprintf("CMAKE_BUILD_TYPE=%s", buildType))
	}

	if runtime.GOOS == "windows" {
		cacheArgs = append(cacheArgs, "-T ClangCL")
	}

	// Configure
	runInDir(t, buildDir, "cmake", append(cacheArgs, testDataDir(t))...)

	// Build
	buildArgs := []string{"--build", "."}
	if buildType != "" {
		buildArgs = append(buildArgs, "--config", buildType)
	}
	buildArgs = append(buildArgs, additionalBuildArgs...)
	if runtime.GOOS == "windows" {
		// CMAKE_VERBOSE_MAKEFILE has no effect on MSBuild, so we have to increase verbosity manually.
		// https://stackoverflow.com/a/70728115/297261
		buildArgs = append(buildArgs, "--", "-clp:ShowCommandLine")
	}
	runInDir(t, buildDir, "cmake", buildArgs...)

	return buildDir
}

func runAndAssertTests(t *testing.T, buildDir string, buildType string, expectedTestStatus map[string]bool) {
	// We expect ctest to exit with 0 if and only if all tests are expected to pass.
	ctestFails := false
	for _, status := range expectedTestStatus {
		if !status {
			ctestFails = true
		}
	}

	junitReportFile := filepath.Join(buildDir, "report.xml")
	runInDirWithExpectedStatus(
		t,
		ctestFails,
		buildDir,
		"ctest",
		"--verbose",
		// Print the output of failed tests to improve the CI logs.
		"--output-on-failure",
		// With a multi-configuration generator (e.g. MSBuild on Windows), ctest requires specifying the configuration.
		// For all other generators, this is a no-op.
		"-C",
		buildType,
		// Instead of parsing CTest's unstructured console output, we let it emit an XML report that contains
		// information on which tests passed or failed.
		"--output-junit",
		junitReportFile,
	)
	require.FileExists(t, junitReportFile)
	junitReportXML, err := os.ReadFile(junitReportFile)
	require.NoError(t, err)
	var junitReport junitTestSuite
	err = xml.Unmarshal(junitReportXML, &junitReport)
	require.NoError(t, err)

	actualTestStatus := make(map[string]bool)
	// Parse the test report in JUnit's XML format to determine which tests passed.
	for _, testCase := range junitReport.TestCases {
		actualTestStatus[testCase.Name] = testCase.Status == "run"
	}
	assert.Equal(t, expectedTestStatus, actualTestStatus)
}

// nolint:unparam
func runInDir(t *testing.T, dir, command string, args ...string) []byte {
	return runInDirWithExpectedStatus(t, false, dir, command, args...)
}

func runInDirWithExpectedStatus(t *testing.T, expectFailure bool, dir string, command string, args ...string) []byte {
	// A timeout of 9 minutes is long enough for all current tests, but stays well under the Go test timeout of
	// 10 minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, command, args...)
	c.Dir = dir
	t.Logf("Working directory: %s", c.Dir)
	t.Logf("Command: %s", c.String())
	out, err := c.Output()
	// Prints compiler command invocations to the test logs.
	t.Log(string(out))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		msg := fmt.Sprintf("%q exited with %d:\nstderr:\n%s", c.String(), exitErr.ExitCode(), string(exitErr.Stderr))
		if !expectFailure {
			require.NoError(t, exitErr, msg)
		} else {
			// When expecting a failure, the command may still fail for an unexpected reason. Thus, in this case, always
			// log the relevant information.
			t.Log(msg)
		}
		return nil
	} else {
		msg := fmt.Sprintf("%q failed to execute with error:%v\n", c.String(), err)
		// Non-ExitErrors or context errors are never expected.
		require.NoError(t, err, msg)
		require.NoError(t, ctx.Err(), msg)
	}
	if expectFailure {
		require.Fail(t, fmt.Sprintf("%q exited with 0:\n%s", c.String(), string(out)))
	}
	return out
}

// extractRuntimeDeps returns the non-system runtime dependencies of the given target as paths relative to buildDir as
// reported by the CMake integration.
func extractRuntimeDeps(t *testing.T, buildDir, target string) []string {
	cmd := exec.Command(
		"cmake",
		"--install", buildDir,
		"--config", cifuzzCmakeBuildType,
		"--component", "cifuzz_internal_deps_"+target,
	)
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			require.NoError(t, exitErr, string(exitErr.Stderr))
		} else {
			require.NoError(t, err)
		}
	}

	lines := strings.Split(string(stdout), "\n")
	var buildDirRelativeDeps []string
	for _, line := range lines {
		// Skip over CMake output.
		if !strings.HasPrefix(line, "-- CIFUZZ ") {
			continue
		}
		statusAndDep := strings.TrimPrefix(line, "-- CIFUZZ ")
		require.Truef(
			t,
			strings.HasPrefix(statusAndDep, "RESOLVED "),
			"Does not start with %q: %q",
			"RESOLVED ",
			statusAndDep,
		)
		absoluteDep := strings.TrimPrefix(statusAndDep, "RESOLVED ")
		// Skip over system deps.
		if !strings.HasPrefix(absoluteDep, buildDir+"/") {
			continue
		}
		relativeDep := strings.TrimPrefix(absoluteDep, buildDir+"/")
		buildDirRelativeDeps = append(buildDirRelativeDeps, relativeDep)
	}

	return buildDirRelativeDeps
}

func testDataDir(t *testing.T) string {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, "testdata")
}

// JUnit XML report format
// See (unofficial source only): https://github.com/windyroad/JUnit-Schema/blob/master/JUnit.xsd
type junitTestSuite struct {
	TestCases []struct {
		Name   string `xml:"name,attr"`
		Status string `xml:"status,attr"`
	} `xml:"testcase"`
}
