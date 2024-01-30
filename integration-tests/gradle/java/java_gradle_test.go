package java

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/gradle"
	"code-intelligence.com/cifuzz/integration-tests/shared"
	gradleBuild "code-intelligence.com/cifuzz/internal/build/java/gradle"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	initCmd "code-intelligence.com/cifuzz/internal/cmd/init"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_Gradle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "gradle")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })
	log.Infof("Project dir: %s", projectDir)

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "com.example.FuzzTestCase::myFuzzTest",
	}

	// Execute the init command
	allStderrLines := cifuzzRunner.Command(t, "init", nil)
	require.NotContains(t, strings.Join(allStderrLines, " "), initCmd.GradleMultiProjectWarningMsg)
	require.FileExists(t, filepath.Join(projectDir, "cifuzz.yaml"))

	// Check that correct error occurs if plugin is missing
	t.Run("runWithoutPlugin", func(t *testing.T) {
		cifuzzRunner.Run(t, &shared.RunOptions{
			ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(gradleBuild.PluginMissingErrorMsg)},
			TerminateAfterExpectedOutput: true,
			ExpectError:                  true,
		})
	})

	// Add the plugin
	linesToAdd := shared.FilterForInstructions(allStderrLines)

	// we only need to add the first filtered line, as it is the gradle plugin
	linesToAdd = linesToAdd[:1]

	shared.AddLinesToFileAtBreakPoint(t, filepath.Join(projectDir, "build.gradle"), linesToAdd, "plugins", true)

	// Execute the create command
	testDir := filepath.Join(
		"src",
		"test",
		"java",
		"com",
		"example",
	)
	err := os.MkdirAll(filepath.Join(projectDir, testDir), 0o755)
	require.NoError(t, err)
	outputPath := filepath.Join(testDir, "FuzzTestCase.java")
	cifuzzRunner.CommandWithFilterForInstructions(t, "create", &shared.CommandOptions{
		Args: []string{"java", "--output", outputPath},
	},
	)

	// Check that the fuzz test was created in the correct directory
	fuzzTestPath := filepath.Join(projectDir, outputPath)
	require.FileExists(t, fuzzTestPath)

	// Check that the findings command doesn't list any findings yet
	findings := shared.GetFindings(t, cifuzz, projectDir)
	require.Empty(t, findings)

	t.Run("runEmptyFuzzTest", func(t *testing.T) {
		// Run the (empty) fuzz test
		cifuzzRunner.Run(t, &shared.RunOptions{
			ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`^paths: \d+`)},
			TerminateAfterExpectedOutput: true,
		})
	})

	// Make the fuzz test call a function
	modifyFuzzTestToCallFunction(t, fuzzTestPath)

	t.Run("run", func(t *testing.T) {
		testRun(t, cifuzzRunner)

		t.Run("htmlCoverageReport", func(t *testing.T) {
			// Produce a jacoco xml coverage report
			testHTMLCoverageReport(t, cifuzz, projectDir)
		})
		t.Run("jacocoCoverageReport", func(t *testing.T) {
			// Produce a jacoco xml coverage report
			testJacocoXMLCoverageReport(t, cifuzz, projectDir)
		})
	})

	t.Run("coverageVSCodePreset", func(t *testing.T) {
		testCoverageVSCodePreset(t, cifuzz, projectDir)
	})

	t.Run("runWithoutFuzzTest", func(t *testing.T) {
		// Run without specifying a fuzz test
		testRunWithoutFuzzTest(t, cifuzzRunner)
	})

	t.Run("runWrongFuzzTest", func(t *testing.T) {
		// Run without specifying a fuzz test
		testRunWrongFuzzTest(t, cifuzzRunner)
	})

	t.Run("runWithAdditionalArgs", func(t *testing.T) {
		// Check if adding additional jazzer parameters via flags is respected
		shared.TestAdditionalJazzerParameters(t, cifuzz, projectDir)
	})

	t.Run("runWithConfigFile", func(t *testing.T) {
		// Check that options set via the config file are respected
		testRunWithConfigFile(t, cifuzzRunner)
	})

	t.Run("bundle", func(t *testing.T) {
		// Run cifuzz bundle and verify the contents of the archive.
		gradle.TestBundleGradle(t, "java", projectDir, cifuzz, "com.example.FuzzTestCase::myFuzzTest")
	})

	t.Run("runWithUpload", func(t *testing.T) {
		testRunWithUpload(t, cifuzzRunner)
	})
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	projectDir := cifuzzRunner.DefaultWorkDir

	// Run the fuzz test
	expectedOutputExp := regexp.MustCompile(`High: Remote Code Execution`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that the findings command lists the finding
	findings := shared.GetFindings(t, cifuzz, projectDir)
	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Details, "Remote Code Execution")

	expectedStackTrace := []*stacktrace.StackFrame{
		{
			SourceFile:  "src/main/java/com/example/ExploreMe.java",
			Line:        19,
			Column:      0,
			FrameNumber: 0,
			Function:    "com.example.ExploreMe.exploreMe",
		},
		{
			SourceFile:  "src/test/java/com/example/FuzzTestCase.java",
			Line:        19,
			Column:      0,
			FrameNumber: 0,
			Function:    "com.example.FuzzTestCase.myFuzzTest",
		},
	}
	require.Equal(t, expectedStackTrace, findings[0].StackTrace)
}

func testRunWithConfigFile(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	projectDir := cifuzzRunner.DefaultWorkDir

	// Check that options set via the config file are respected
	configFileContent := "print-json: true"
	err := os.WriteFile(filepath.Join(projectDir, "cifuzz.yaml"), []byte(configFileContent), 0o644)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clear cifuzz.yml so that subsequent tests run with defaults
		err = os.WriteFile(filepath.Join(projectDir, "cifuzz.yaml"), nil, 0o644)
		assert.NoError(t, err)
	})

	expectedOutputExp := regexp.MustCompile(`"finding": {`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	t.Run("WithFlags", func(t *testing.T) {
		// Check that command-line flags take precedence over config file
		// settings
		cifuzzRunner.Run(t, &shared.RunOptions{
			Args:             []string{"--json=false"},
			UnexpectedOutput: expectedOutputExp,
		})
	})
}

func testHTMLCoverageReport(t *testing.T, cifuzz, dir string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "report", "--format", "html", "com.example.FuzzTestCase::myFuzzTest")
	cmd.Dir = dir
	log.Printf("Command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output),
		fmt.Sprintf("Created coverage HTML report: %s", filepath.Join("report", "html")),
	)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "html", "index.html")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for
	// com.example
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	report := string(reportBytes)
	require.Contains(t, report, "com.example")
}

func testJacocoXMLCoverageReport(t *testing.T, cifuzz, dir string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "report", "--format", "jacocoxml", "com.example.FuzzTestCase::myFuzzTest")
	cmd.Dir = dir
	log.Printf("Command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output),
		fmt.Sprintf("Created jacoco.xml coverage report: %s", filepath.Join("report", "jacoco.xml")),
	)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "jacoco.xml")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for
	// ExploreMe.java source file, but not for App.java.
	reportFile, err := os.Open(reportPath)
	require.NoError(t, err)
	defer reportFile.Close()
	summary := coverage.ParseJacocoXMLIntoSummary(reportFile)
	for _, file := range summary.Files {
		if file.Filename == "com/example/ExploreMe.java" {
			assert.Equal(t, 2, file.Coverage.FunctionsHit)
			assert.Equal(t, 10, file.Coverage.LinesHit)
			assert.Equal(t, 8, file.Coverage.BranchesHit)

		} else if file.Filename == "com/example/App.java" {
			assert.Equal(t, 0, file.Coverage.FunctionsHit)
			assert.Equal(t, 0, file.Coverage.LinesHit)
			assert.Equal(t, 0, file.Coverage.BranchesHit)
		}
	}
}

func testCoverageVSCodePreset(t *testing.T, cifuzz, dir string) {
	cmd := executil.Command(cifuzz, "coverage",
		"-v",
		"--preset=vscode",
		"com.example.FuzzTestCase::myFuzzTest")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "jacoco.xml")
	require.FileExists(t, reportPath)
}

// modifyFuzzTestToCallFunction modifies the fuzz test stub created by `cifuzz create` to actually call a function.
func modifyFuzzTestToCallFunction(t *testing.T, fuzzTestPath string) {
	f, err := os.OpenFile(fuzzTestPath, os.O_RDWR, 0o700)
	require.NoError(t, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)

	var lines []string
	var seenBeginningOfFuzzTestFunc bool
	var addedFunctionCall bool
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "    @FuzzTest") {
			seenBeginningOfFuzzTestFunc = true
		}
		// Insert the function call at the end of the myFuzzTest
		// function, right above the "}".
		if seenBeginningOfFuzzTestFunc && strings.HasPrefix(scanner.Text(), "    }") {
			lines = append(lines, []string{
				"        int a = data.consumeInt();",
				"        int b = data.consumeInt();",
				"        String c = data.consumeRemainingAsString();",
				"		 ExploreMe ex = new ExploreMe(a);",
				"        ex.exploreMe(b, c);",
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

func testRunWithUpload(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	shared.TestRunWithUpload(t, testdata, cifuzz, "com.example.FuzzTestCase::myFuzzTest")
}

func testRunWithoutFuzzTest(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// Run without specifying a fuzz test
	expectedOutputExp := regexp.MustCompile(`High: Remote Code Execution`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		FuzzTest:        "",
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})
}

func testRunWrongFuzzTest(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	expectedOutputExp := regexp.MustCompile(`Invalid usage:`)

	// Run with wrong class name
	cifuzzRunner.Run(t, &shared.RunOptions{
		FuzzTest:        "com.example.WrongFuzzTestCase",
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
		ExpectError:     true,
	})

	// Run with wrong method name
	cifuzzRunner.Run(t, &shared.RunOptions{
		FuzzTest:        "com.example.FuzzTestCase::wrongFuzzTest",
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
		ExpectError:     true,
	})
}
