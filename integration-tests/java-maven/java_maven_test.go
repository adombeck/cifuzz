package maven

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mattn/go-zglob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_Maven(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "maven")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })
	log.Infof("Project dir: %s", projectDir)

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "com.example.FuzzTestCase::myFuzzTest",
	}

	// Execute the init command
	// The instructions file for maven includes dependencies, plugin and a profile section for jacoco that
	// need to be included at different locations in the pom.xml, so we split the instructions file up into
	// <dependency> and <profile> and ignore the code adding the jacoco plugin because the user should already
	// have it, the instruction is just a nudge so it doesn't get overlooked and doesn't need to be tested.
	linesToAdd := cifuzzRunner.CommandWithFilterForInstructions(t, "init", nil)
	assert.FileExists(t, filepath.Join(projectDir, "cifuzz.yaml"))
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		strings.Split(strings.Split(strings.Join(linesToAdd, "\n"), "<plugin>")[0], "\n"),
		"\t</dependencies>",
		false,
	)
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		strings.Split("<profile>"+strings.Split(strings.Join(linesToAdd, "\n"), "<profile>")[1], "\n"),
		"\t</profiles>",
		false,
	)

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
		testBundle(t, projectDir, cifuzz, "com.example.FuzzTestCase::myFuzzTest")
	})

	t.Run("runWithUpload", func(t *testing.T) {
		testRunWithUpload(t, cifuzzRunner)
	})

	t.Run("containerRun", func(t *testing.T) {
		testContainerRun(t, cifuzzRunner)
	})
}

func testHTMLCoverageReport(t *testing.T, cifuzz, dir string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "report", "--format", "html", "com.example.FuzzTestCase::myFuzzTest")
	cmd.Dir = dir
	log.Printf("Command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output), "Created coverage HTML report: report")

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "index.html")
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

func testBundle(t *testing.T, dir string, cifuzz string, args ...string) {
	tempDir := testutil.MkdirTemp(t, "", "cifuzz-archive-*")
	bundlePath := filepath.Join(tempDir, "fuzz_tests.tar.gz")
	t.Logf("creating test bundle in %s", tempDir)

	// Create a dictionary
	dictPath := filepath.Join(tempDir, "some_dict")
	err := os.WriteFile(dictPath, []byte("test-dictionary-content"), 0o600)
	require.NoError(t, err)

	// Create a seed corpus directory with an empty seed
	seedCorpusDir, err := os.MkdirTemp(tempDir, "seeds-")
	require.NoError(t, err)
	err = fileutil.Touch(filepath.Join(seedCorpusDir, "empty"))
	require.NoError(t, err)

	defaultArgs := []string{
		"bundle",
		"-o", bundlePath,
		"--dict", dictPath,
		"--seed-corpus", seedCorpusDir,
		"--timeout", "100m",
		"--branch", "my-branch",
		"--commit", "123456abcdef",
		"--docker-image", "my-image",
		"--env", "FOO=foo",
		// This should be set to the value from the local environment,
		// which we set to "bar" below
		"--env", "BAR",
		// This should be ignored because it's not set in the local
		// environment
		"--env", "NO_SUCH_VARIABLE",
		"--verbose",
	}

	args = append(defaultArgs, args...)
	metadata, archiveDir := shared.TestRunBundle(t, dir, cifuzz, bundlePath, os.Environ(), args...)
	defer fileutil.Cleanup(archiveDir)

	// Verify code revision given by `--branch` and `--commit-sha` flags
	assert.Equal(t, "my-branch", metadata.CodeRevision.Git.Branch)
	assert.Equal(t, "123456abcdef", metadata.CodeRevision.Git.Commit)

	// Verify that the metadata contains the Docker image
	assert.Equal(t, "my-image", metadata.RunEnvironment.Docker)

	// Verify the metadata contains the env vars
	require.Equal(t, []string{"FOO=foo", "BAR=bar"}, metadata.Fuzzers[0].EngineOptions.Env)

	// Verify that the metadata contains one fuzzer
	require.Equal(t, 1, len(metadata.Fuzzers))
	fuzzerMetadata := metadata.Fuzzers[0]

	// Verify that name is set
	assert.Equal(t, fuzzerMetadata.Name, "com.example.FuzzTestCase::myFuzzTest")

	// Verify that the dictionary has been packaged with the fuzzer
	dictPath = filepath.Join(archiveDir, fuzzerMetadata.Dictionary)
	require.FileExists(t, dictPath)
	content, err := os.ReadFile(dictPath)
	require.NoError(t, err)
	assert.Equal(t, "test-dictionary-content", string(content))

	// Verify that the seed corpus has been packaged with the fuzzer
	seedCorpusPath := filepath.Join(archiveDir, fuzzerMetadata.Seeds)
	require.DirExists(t, seedCorpusPath)

	// Verify that runtime dependencies have been packed
	jarPattern := filepath.Join(archiveDir, "runtime_deps", "*.jar")
	jarMatches, err := zglob.Glob(jarPattern)
	require.NoError(t, err)
	assert.Equal(t, 19, len(jarMatches))

	classPattern := filepath.Join(archiveDir, "runtime_deps", "**", "*.class")
	classMatches, err := zglob.Glob(classPattern)
	require.NoError(t, err)
	assert.Equal(t, 3, len(classMatches))

	// Verify that the manifest.jar has been created
	manifestJARPath := filepath.Join(archiveDir, "com.example.FuzzTestCase_myFuzzTest", "manifest.jar")
	require.FileExists(t, manifestJARPath)

	// Verify contents of manifest.jar
	extractedManifestPath := filepath.Join(archiveDir, "manifest")
	err = archiveutil.Unzip(manifestJARPath, extractedManifestPath)
	require.NoError(t, err)
	manifestFilePath := filepath.Join(extractedManifestPath, "META-INF", "MANIFEST.MF")
	require.FileExists(t, manifestFilePath)
	content, err = os.ReadFile(manifestFilePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Jazzer-Target-Class: com.example.FuzzTestCase")
	assert.Contains(t, string(content), "Jazzer-Target-Method: myFuzzTest")
	assert.Contains(t, string(content), "Jazzer-Fuzz-Target-Class: com.example.FuzzTestCase")

	// Verify that the source_map.json has been created
	sourceMapPath := filepath.Join(archiveDir, "source_map.json")
	require.FileExists(t, sourceMapPath)

	// Verify contents of source_map.json
	content, err = os.ReadFile(sourceMapPath)
	require.NoError(t, err)
	sourceMap := sourcemap.SourceMap{}
	err = json.Unmarshal(content, &sourceMap)
	require.NoError(t, err)
	assert.Equal(t, 1, len(sourceMap.JavaPackages))

	expectedSourceLocations := []string{
		"src/test/java/com/example/FuzzTestCase.java",
		"src/main/java/com/example/ExploreMe.java",
		"src/main/java/com/example/App.java",
	}
	assert.Equal(t, len(expectedSourceLocations), len(sourceMap.JavaPackages["com.example"]))
	for _, expectedSourceLocation := range expectedSourceLocations {
		assert.Contains(t, sourceMap.JavaPackages["com.example"], expectedSourceLocation)
	}
}

func testRunWithConfigFile(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	testData := cifuzzRunner.DefaultWorkDir

	configFileContent := "print-json: true"
	err := os.WriteFile(filepath.Join(testData, "cifuzz.yaml"), []byte(configFileContent), 0o644)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clear cifuzz.yml so that subsequent tests run with defaults (e.g. sandboxing).
		err = os.WriteFile(filepath.Join(testData, "cifuzz.yaml"), nil, 0o644)
		require.NoError(t, err)
	})

	expectedOutputExp := regexp.MustCompile(`"finding": {`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that command-line flags take precedence over config file
	// settings (only on Linux because we only support Minijail on
	// Linux).
	cifuzzRunner.Run(t, &shared.RunOptions{
		Args:             []string{"--json=false"},
		UnexpectedOutput: expectedOutputExp,
	})
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testData := cifuzzRunner.DefaultWorkDir

	// Run the fuzz test
	expectedOutputExp := regexp.MustCompile(`High: Remote Code Execution`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that the findings command lists the finding
	findings := shared.GetFindings(t, cifuzz, testData)
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

func testContainerRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	shared.TestContainerRun(t, cifuzzRunner, "", &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{
			regexp.MustCompile(`High: Remote Code Execution`),
		},
	})
}
