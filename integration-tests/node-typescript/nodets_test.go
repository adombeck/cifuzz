package nodets

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_NodeTS_InitCreateRunCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "nodets")
	defer fileutil.Cleanup(projectDir)

	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "FuzzTestCase",
	}

	// Execute the init command
	instructions := cifuzzRunner.CommandWithFilterForInstructions(t, "init", &shared.CommandOptions{
		Args: []string{"ts"},
	})
	require.FileExists(t, filepath.Join(projectDir, "cifuzz.yaml"))

	// Execute npm install --save-dev @jazzer.js/jest-runner
	npmArgs := getNpmArgs(t, instructions)
	require.NotEmpty(t, npmArgs)

	cmd := exec.Command("npm", npmArgs...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	t.Logf("Command: %s", cmd.String())
	err := cmd.Run()
	require.NoError(t, err)

	// Create jest.config.ts file
	jestConfig := getJestConfig(t, instructions)
	jestConfigPath := filepath.Join(projectDir, "jest.config.ts")
	err = os.WriteFile(jestConfigPath, []byte(jestConfig), 0o644)
	require.NoError(t, err)

	// Execute the create command
	fuzzTestPath := filepath.Join(projectDir, "FuzzTestCase.fuzz.ts")
	cifuzzRunner.CommandWithFilterForInstructions(t, "create", &shared.CommandOptions{
		Args: []string{"ts", "--output", fuzzTestPath},
	},
	)

	// Check that the fuzz test was created in the correct directory
	require.FileExists(t, fuzzTestPath)

	// Check that the findings command doesn't list any findings yet
	findings := shared.GetFindings(t, cifuzz, projectDir)
	assert.Empty(t, findings)

	// Run the (empty) fuzz test
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`^paths: \d+`)},
		TerminateAfterExpectedOutput: true,
	})

	// Make the fuzz test call a function
	modifyFuzzTestToCallFunction(t, fuzzTestPath)

	t.Run("run", func(t *testing.T) {
		testRun(t, &cifuzzRunner)
	})
	t.Run("htmlReport", func(t *testing.T) {
		// Produce a coverage report for parser_fuzz_test
		testHTMLCoverageReport(t, cifuzz, projectDir, "FuzzTestCase")
	})
	t.Run("lcovReport", func(t *testing.T) {
		// Produces a coverage report for crashing_fuzz_test
		testLcovCoverageReport(t, cifuzz, projectDir, "FuzzTestCase")
	})
	t.Run("runWithUpload", func(t *testing.T) {
		testRunWithUpload(t, &cifuzzRunner)
	})
}

func getNpmArgs(t *testing.T, instructions []string) []string {
	t.Helper()

	for _, instruction := range instructions {
		if strings.HasPrefix(instruction, "npm") {
			args := strings.TrimSpace(strings.TrimPrefix(instruction, "npm"))
			return strings.Split(args, " ")
		}
	}

	return nil
}

func getJestConfig(t *testing.T, instructions []string) string {
	t.Helper()

	for i, instruction := range instructions {
		if strings.HasPrefix(instruction, "import type") {
			return strings.Join(instructions[i:len(instructions)-1], "\n")
		}
	}

	return ""
}

// modifyFuzzTestToCallFunction modifies the fuzz test stub created by `cifuzz create` to actually call a function.
func modifyFuzzTestToCallFunction(t *testing.T, fuzzTestPath string) {
	f, err := os.OpenFile(fuzzTestPath, os.O_RDWR, 0o700)
	require.NoError(t, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// At the top of the file we add the required headers
	lines := []string{
		`import { exploreMe } from "./ExploreMe";`,
	}
	var seenBeginningOfFuzzTestFunc bool
	var addedFunctionCall bool
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "test.fuzz(") {
			seenBeginningOfFuzzTestFunc = true
		}
		// Insert the function call at the end of the FUZZ_TEST
		// function, right above the "}".
		if seenBeginningOfFuzzTestFunc && strings.HasPrefix(scanner.Text(), "}") {
			lines = append(lines, []string{
				"  const provider: FuzzedDataProvider = new FuzzedDataProvider(data);",
				"  const a: number = provider.consumeNumber();",
				"  const b: number = provider.consumeNumber();",
				"  const c: string = provider.consumeString(8);",
				"  exploreMe(a, b, c);",
			}...)
			addedFunctionCall = true
		}
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())
	require.True(t, addedFunctionCall)

	// Write the new content of the fuzz test back to file
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = f.WriteString(strings.Join(lines, "\n"))
	require.NoError(t, err)
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// Run the fuzz test
	expectedOutputExp := regexp.MustCompile("Crash!")
	// setting -max_len to 8192 in ".jazzerjsrc" file to test if the file is read and picked up
	unexpectedOutputExp := regexp.MustCompile("INFO: -max_len is not provided; libFuzzer will not generate inputs larger than 4096 bytes")
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs:  []*regexp.Regexp{expectedOutputExp},
		UnexpectedOutput: unexpectedOutputExp,
	})

	// Check that the findings command lists the finding
	findings := shared.GetFindings(t, cifuzzRunner.CIFuzzPath, cifuzzRunner.DefaultWorkDir)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Details, "Crash")

	expectedStackTrace := []*stacktrace.StackFrame{
		{
			SourceFile:  "ExploreMe.ts",
			Line:        6,
			Column:      12,
			FrameNumber: 0,
			Function:    "exploreMe",
		},
		{
			SourceFile:  "FuzzTestCase.fuzz.ts",
			Line:        19,
			Column:      12,
			FrameNumber: 0,
			Function:    "",
		},
	}
	assert.Equal(t, expectedStackTrace, findings[0].StackTrace)

	// Check that options set via the config file are respected
	configFileContent := "print-json: true"
	err := os.WriteFile(filepath.Join(cifuzzRunner.DefaultWorkDir, "cifuzz.yaml"), []byte(configFileContent), 0o644)
	require.NoError(t, err)
	expectedOutputExp = regexp.MustCompile(`"finding": {`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that command-line flags take precedence over config file settings
	cifuzzRunner.Run(t, &shared.RunOptions{
		Args:             []string{"--json=false"},
		UnexpectedOutput: expectedOutputExp,
	})

	// Clear cifuzz.yml so that subsequent tests run with defaults (e.g. sandboxing).
	err = os.WriteFile(filepath.Join(cifuzzRunner.DefaultWorkDir, "cifuzz.yaml"), nil, 0o644)
	require.NoError(t, err)
}

func testHTMLCoverageReport(t *testing.T, cifuzz, dir, fuzzTest string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "coverage-report", fuzzTest)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "coverage-report", "lcov-report", "index.html")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for ExploreMe.ts
	// and FuzzTestCase.fuzz.ts
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	report := string(reportBytes)
	assert.Contains(t, report, "ExploreMe.ts")
	assert.Contains(t, report, "FuzzTestCase.fuzz.ts")
}

func testLcovCoverageReport(t *testing.T, cifuzz, dir, fuzzTest string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--format=lcov", "--output", "coverage-report", fuzzTest)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "coverage-report", "lcov.info")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains the right coverage
	// for both source files
	reportFile, err := os.Open(reportPath)
	require.NoError(t, err)
	defer reportFile.Close()
	summary, err := coverage.ParseLCOVReportIntoSummary(reportFile)
	require.NoError(t, err)
	assert.Equal(t, 2, len(summary.Files))
	for _, file := range summary.Files {
		if file.Filename == "ExploreMe.ts" {
			assert.Equal(t, 1, file.Coverage.FunctionsHit)
			assert.Equal(t, 6, file.Coverage.LinesHit)
			// How many of the 4 branches that don't lead to a crash are hit
			// depends on how lucky the fuzzer is, so we just check that
			// it's between 4 and 8.
			assert.GreaterOrEqual(t, file.Coverage.BranchesHit, 4)
			assert.LessOrEqual(t, file.Coverage.BranchesHit, 8)
		} else if file.Filename == "FuzzTestCase.fuzz.ts" {
			assert.Equal(t, 1, file.Coverage.FunctionsHit)
			assert.Equal(t, 8, file.Coverage.LinesHit)
			assert.Equal(t, 0, file.Coverage.BranchesHit)
		}
	}
}

func testRunWithUpload(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdir := cifuzzRunner.DefaultWorkDir
	shared.TestRunWithUpload(t, testdir, cifuzz, "FuzzTestCase")
}
