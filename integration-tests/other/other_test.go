package other

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

var installDir string

func TestMain(m *testing.M) {
	defer fileutil.Cleanup(installDir)
	m.Run()
}

func TestIntegration_Other(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if runtime.GOOS == "windows" {
		t.Skip("Other build systems are currently not supported on Windows")
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir = shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Setup testdata
	testdata := shared.CopyTestdataDir(t, "other")
	t.Logf("executing other build system integration test in %s", testdata)

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  testdata,
		DefaultFuzzTest: "my_fuzz_test",
	}

	t.Run("runBuildOnly", func(t *testing.T) {
		cifuzzRunner.Run(t, &shared.RunOptions{
			Args: []string{"--build-only"},
			Env:  cifuzzEnv(testdata),
		})
	})

	t.Run("run", func(t *testing.T) {
		testRun(t, cifuzzRunner)

		t.Run("htmlReport", func(t *testing.T) {
			// Produce a coverage report for my_fuzz_test
			testHTMLCoverageReport(t, cifuzz, testdata)
		})

		t.Run("lcovReport", func(t *testing.T) {
			// Produce a coverage report for my_fuzz_test
			testLcovCoverageReport(t, cifuzz, testdata)
		})
	})

	t.Run("runWithDefaultDict", func(t *testing.T) {
		testRunWithDefaultDict(t, cifuzzRunner)
	})

	t.Run("bundle", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("Creating a bundle for other build systems is currently only supported on Linux")
		}

		// Use a different Makefile on macOS, because shared objects need
		// to be built differently there
		args := []string{"my_fuzz_test", "--build-command", buildCommand()}

		// Run cifuzz bundle and verify the contents of the archive.
		shared.TestBundleLibFuzzer(t, testdata, cifuzz, cifuzzEnv(testdata), args...)
	})

	t.Run("containerRun", func(t *testing.T) {
		if runtime.GOOS != "linux" && !config.AllowUnsupportedPlatforms() {
			t.Skip("Creating a bundle for other build systems (which is required by the container run command) is currently only supported on Linux")
		}
		testContainerRun(t, cifuzzRunner)
	})
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir

	expectedOutputs := []*regexp.Regexp{
		regexp.MustCompile(`^==\d*==ERROR: AddressSanitizer: heap-buffer-overflow`),
	}

	// Check that Minijail is used (if running on Linux, because Minijail
	// is only supported on Linux)
	if runtime.GOOS == "linux" {
		expectedOutputs = append(expectedOutputs, regexp.MustCompile(`bin/minijail0`))
	}

	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: expectedOutputs,
		Env:             cifuzzEnv(testdata),
		Args:            []string{"--build-command", buildCommand()},
	})

	// Check that the findings command lists the findings
	findings := shared.GetFindings(t, cifuzz, testdata)
	require.Len(t, findings, 2)
	var asanFinding *finding.Finding
	var ubsanFinding *finding.Finding
	for _, f := range findings {
		if strings.HasPrefix(f.Details, "heap-buffer-overflow") {
			asanFinding = f
		} else if strings.HasPrefix(f.Details, "undefined behavior") {
			ubsanFinding = f
		} else {
			t.Fatalf("unexpected finding: %q", f.Details)
		}
	}

	// Verify that there is an ASan finding and that it has the correct details.
	require.NotNil(t, asanFinding)
	// Verify that ASan findings come with inputs under the project directory.
	require.NotEmpty(t, asanFinding.InputFile)
	require.False(t, filepath.IsAbs(asanFinding.InputFile), "Should be relative: %s", asanFinding.InputFile)
	require.FileExists(t, filepath.Join(testdata, asanFinding.InputFile))
	// TODO: This check currently fails on macOS because there
	// llvm-symbolizer doesn't read debug info from object files.
	// See https://github.com/google/sanitizers/issues/207#issuecomment-136495556
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/bug/trigger_bugs.cpp",
				Line:        11,
				Column:      3,
				FrameNumber: 1,
				Function:    "triggerASan",
			},
			{
				SourceFile:  "src/explore/explore_me.cpp",
				Line:        10,
				Column:      11,
				FrameNumber: 2,
				Function:    "exploreMe",
			},
			{
				SourceFile:  "my_fuzz_test.cpp",
				Line:        18,
				Column:      3,
				FrameNumber: 3,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}

		require.Equal(t, expectedStackTrace, asanFinding.StackTrace)
	}

	// Verify that there is a UBSan finding and that it has the correct details.
	require.NotNil(t, ubsanFinding)
	// Verify that UBSan findings come with inputs under the project directory.
	require.NotEmpty(t, ubsanFinding.InputFile)
	require.False(t, filepath.IsAbs(ubsanFinding.InputFile), "Should be relative: %s", ubsanFinding.InputFile)
	require.FileExists(t, filepath.Join(testdata, ubsanFinding.InputFile))
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/bug/trigger_bugs.cpp",
				Line:        18,
				Column:      5,
				FrameNumber: 0,
				Function:    "triggerUBSan",
			},
			{
				SourceFile:  "src/explore/explore_me.cpp",
				Line:        13,
				Column:      9,
				FrameNumber: 1,
				Function:    "exploreMe",
			},
			{
				SourceFile:  "my_fuzz_test.cpp",
				Line:        18,
				Column:      3,
				FrameNumber: 2,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}
		require.Equal(t, expectedStackTrace, ubsanFinding.StackTrace)
	}
}

func testHTMLCoverageReport(t *testing.T, cifuzz string, dir string) {
	t.Helper()
	var err error

	fuzzTest := "my_fuzz_test"

	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "coverage-report",
		"--build-command", buildCommand(),
		fuzzTest)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env, err = envutil.Copy(os.Environ(), cifuzzEnv(dir))
	require.NoError(t, err)
	t.Logf("Command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	err = cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "coverage-report", "explore", "index.html")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for the api.cpp
	// source file, but not for our headers.
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	report := string(reportBytes)
	require.Contains(t, report, "explore_me.cpp")
	require.NotContains(t, report, "include/cifuzz")
}

func testLcovCoverageReport(t *testing.T, cifuzz string, dir string) {
	t.Helper()
	var err error

	fuzzTest := "crashing_fuzz_test"
	reportPath := filepath.Join(dir, fuzzTest+".lcov")

	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--format=lcov",
		"--output", reportPath,
		"--build-command", buildCommand(),
		fuzzTest)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env, err = envutil.Copy(os.Environ(), cifuzzEnv(dir))
	require.NoError(t, err)
	err = cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	require.FileExists(t, reportPath)

	// Read the report and extract all uncovered lines in the fuzz test source file.
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	lcov := bufio.NewScanner(bytes.NewBuffer(reportBytes))
	isFuzzTestSource := false
	var uncoveredLines []uint
	for lcov.Scan() {
		line := lcov.Text()

		if strings.HasPrefix(line, "SF:") {
			if strings.HasSuffix(line, "/crashing_fuzz_test.c") {
				isFuzzTestSource = true
			} else {
				isFuzzTestSource = false
				assert.Fail(t, "Unexpected source file: "+line)
			}
		}

		if !isFuzzTestSource || !strings.HasPrefix(line, "DA:") {
			continue
		}
		split := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
		require.Len(t, split, 2)
		if split[1] == "0" {
			lineNo, err := strconv.Atoi(split[0])
			require.NoError(t, err)
			uncoveredLines = append(uncoveredLines, uint(lineNo))
		}
	}

	assert.Subset(t, []uint{
		// Lines after the three crashes. Whether these are covered depends on implementation details of the coverage
		// instrumentation, so we conservatively assume they aren't covered.
		21, 31, 41,
	},
		uncoveredLines)
}

func testRunWithDefaultDict(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{
			regexp.MustCompile(`Dictionary: 1 entries`),
		},
		Env:  cifuzzEnv(cifuzzRunner.DefaultWorkDir),
		Args: []string{"--build-command", buildCommand()},
	})
}

func testContainerRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	tag := "cifuzz-test-container-run-other:latest"

	shared.BuildDockerImage(t, tag, cifuzzRunner.DefaultWorkDir)
	shared.TestContainerRun(t, cifuzzRunner, tag, &shared.RunOptions{
		Env: cifuzzEnv(cifuzzRunner.DefaultWorkDir),
		ExpectedOutputs: []*regexp.Regexp{
			regexp.MustCompile(`^==\d*==ERROR: AddressSanitizer: heap-buffer-overflow`),
			regexp.MustCompile(`^SUMMARY: UndefinedBehaviorSanitizer: undefined-behavior`),
		},
	})
}

func cifuzzEnv(workDir string) []string {
	if runtime.GOOS == "linux" {
		return []string{"LD_LIBRARY_PATH=" + filepath.Join(workDir, "build")}
	} else if runtime.GOOS == "darwin" {
		return []string{"DYLD_LIBRARY_PATH=" + workDir}
	}
	return nil
}

func buildCommand() string {
	if runtime.GOOS == "darwin" {
		return "make -f Makefile.darwin clean && make -f Makefile.darwin $FUZZ_TEST"
	}
	return "make clean && make $FUZZ_TEST"
}
