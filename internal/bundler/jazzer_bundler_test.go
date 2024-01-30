package bundler

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestAssembleArtifactsJava_Fuzzing(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	projectDir := filepath.Join("testdata", "jazzer", "project")

	fuzzTests := []string{"com.example.FuzzTest", "com.example.AnotherFuzzTest"}
	targetMethods := []string{"FuzzTestCase", "AnotherFuzzTestCase"}

	runtimeDeps := []string{
		// A library in the project's build directory.
		filepath.Join(projectDir, "lib", "mylib.jar"),
		// a directory structure of class files
		filepath.Join(projectDir, "src", "main"),
		filepath.Join(projectDir, "src", "test"),
	}

	bundle, err := os.CreateTemp("", "bundle-archive-")
	require.NoError(t, err)
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewTarArchiveWriter(bufWriter, true)

	b := newJazzerBundler(&Opts{
		Env:        []string{"FOO=foo"},
		ProjectDir: projectDir,
		tempDir:    tempDir,
	}, archiveWriter)
	fuzzers, err := b.assembleArtifacts(fuzzTests, targetMethods, runtimeDeps)
	require.NoError(t, err)

	err = archiveWriter.Close()
	require.NoError(t, err)
	err = bufWriter.Flush()
	require.NoError(t, err)
	err = bundle.Close()
	require.NoError(t, err)

	// we expect forward slashes even on windows, see also:
	// TestAssembleArtifactsJava_WindowsForwardSlashes
	expectedDeps := []string{
		// manifest.jar should always be first element in runtime paths
		"com.example.FuzzTest_FuzzTestCase/manifest.jar",
		"runtime_deps/mylib.jar",
		"runtime_deps/src/main",
		"runtime_deps/src/test",
	}
	expectedFuzzer := &archive.Fuzzer{
		Name:         "com.example.FuzzTest::FuzzTestCase",
		Engine:       "JAVA_LIBFUZZER",
		ProjectDir:   b.opts.ProjectDir,
		RuntimePaths: expectedDeps,
		EngineOptions: archive.EngineOptions{
			Env:   b.opts.Env,
			Flags: b.opts.EngineArgs,
		},
	}
	require.Equal(t, 2, len(fuzzers))
	require.Equal(t, *expectedFuzzer, *fuzzers[0])

	// Unpack archive contents with tar.

	out := testutil.MkdirTemp(t, "", "bundler-test-*")
	cmd := exec.Command("tar", "-xvf", bundle.Name(), "-C", out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("Command: %v", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)

	// Check that the archive has the expected contents
	expectedContents, err := listFilesRecursively(filepath.Join("testdata", "jazzer", "expected-archive-contents"))
	require.NoError(t, err)
	actualContents, err := listFilesRecursively(out)
	require.NoError(t, err)
	require.Equal(t, expectedContents, actualContents)
}

func listFilesRecursively(dir string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		paths = append(paths, relPath)
		return nil
	})
	return paths, errors.WithMessagef(err, "Failed to list files from directory %s", dir)
}

// As long as we only have linux based runner we should make sure
// that the runtime paths are using forward slashes even if the
// bundle was created on windows
func TestAssembleArtifactsJava_WindowsForwardSlashes(t *testing.T) {
	projectDir := filepath.Join("testdata", "jazzer", "project")
	runtimeDeps := []string{
		filepath.Join(projectDir, "lib", "mylib.jar"),
	}
	fuzzTests := []string{"com.example.FuzzTest"}
	targetMethods := []string{"FuzzTestCase"}

	bundle, err := os.CreateTemp("", "bundle-archive-")
	require.NoError(t, err)
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewTarArchiveWriter(bufWriter, true)
	t.Cleanup(func() {
		archiveWriter.Close()
		bufWriter.Flush()
		bundle.Close()
	})

	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	b := newJazzerBundler(&Opts{
		tempDir:    tempDir,
		ProjectDir: projectDir,
	}, archiveWriter)

	fuzzers, err := b.assembleArtifacts(fuzzTests, targetMethods, runtimeDeps)
	require.NoError(t, err)

	for _, fuzzer := range fuzzers {
		for _, runtimePath := range fuzzer.RuntimePaths {
			assert.NotContains(t, runtimePath, "\\")
		}
	}
}

// Testing a gradle project with two fuzz tests in one class
// and a custom source directory for tests
func TestIntegration_GradleCustomSrcMultipeTests(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// copy test data project to temp dir
	testProject := filepath.Join("testdata", "jazzer", "gradle", "multi-custom")
	projectDir := shared.CopyCustomTestdataDir(t, testProject, "gradle")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })

	tempDir := testutil.MkdirTemp(t, "", "cifuzz-archive-*")
	bundlePath := filepath.Join(tempDir, "fuzz_tests.tar.gz")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))
	args := []string{
		"bundle",
		"-o", bundlePath,
	}
	metadata, _ := shared.TestRunBundle(t, filepath.Join(projectDir, "testsuite"), cifuzz, bundlePath, os.Environ(), args...)

	// result should contain two fuzz tests from one class
	// Verify that the metadata contains one fuzzer
	require.Equal(t, 2, len(metadata.Fuzzers))
	// result should contain fuzz tests with fully qualified names
	assert.Equal(t, "com.example.TestCases::myFuzzTest1", metadata.Fuzzers[0].Name)
	assert.Equal(t, "com.example.TestCases::myFuzzTest2", metadata.Fuzzers[1].Name)
}

func TestCreateManifestJar_TargetMethod(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-temp-dir-")
	jazzerBundler := jazzerBundler{
		opts: &Opts{
			tempDir: tempDir,
		},
	}
	targetClass := "com.example.FuzzTestCase"
	targetMethod := "myFuzzTest"
	jarPath, err := jazzerBundler.createManifestJar(targetClass, targetMethod)
	require.NoError(t, err)

	err = archiveutil.Unzip(jarPath, tempDir)
	require.NoError(t, err)
	manifestPath := filepath.Join(tempDir, "META-INF", "MANIFEST.MF")
	require.FileExists(t, manifestPath)
	content, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetClassManifest, targetClass))
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetClassManifestLegacy, targetClass))
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetMethodManifest, targetMethod))
}

func TestAssembleArtifacts_TargetMethodValidPath(t *testing.T) {
	projectDir := filepath.Join("testdata", "jazzer", "project")
	fuzzTests := []string{"com.example.FuzzTest"}
	targetMethods := []string{"myFuzzTest"}

	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	b := newJazzerBundler(&Opts{
		tempDir:    tempDir,
		ProjectDir: projectDir,
	}, &archive.NullArchiveWriter{})

	fuzzers, err := b.assembleArtifacts(fuzzTests, targetMethods, nil)
	require.NoError(t, err)

	require.Len(t, fuzzers, 1)
	require.Len(t, fuzzers[0].RuntimePaths, 1)
	assert.Contains(t, fuzzers[0].RuntimePaths[0], "com.example.FuzzTest_myFuzzTest")
	assert.Equal(t, fuzzers[0].Name, "com.example.FuzzTest::myFuzzTest")
}

func TestBundleAllFuzzTests(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testCases := []struct {
		fuzzTargets       []string
		expectedFuzzTests []string
	}{
		{ // No fuzz tests specified
			fuzzTargets: nil,
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase1::myFuzzTest",
				"com.example.FuzzTestCase2::oneFuzzTest",
				"com.example.FuzzTestCase2::anotherFuzzTest",
			},
		},
		{ // One class specified that only has one method
			fuzzTargets:       []string{"com.example.FuzzTestCase1"},
			expectedFuzzTests: []string{"com.example.FuzzTestCase1::myFuzzTest"},
		},
		{ // One class specified that has two methods
			fuzzTargets: []string{"com.example.FuzzTestCase2"},
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase2::oneFuzzTest",
				"com.example.FuzzTestCase2::anotherFuzzTest"},
		},
		{ // One class with target method specified
			fuzzTargets:       []string{"com.example.FuzzTestCase2::anotherFuzzTest"},
			expectedFuzzTests: []string{"com.example.FuzzTestCase2::anotherFuzzTest"},
		},
		{ // Two classes specified, one with target method one without
			fuzzTargets: []string{"" +
				"com.example.FuzzTestCase1",
				"com.example.FuzzTestCase2::anotherFuzzTest"},
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase1::myFuzzTest",
				"com.example.FuzzTestCase2::anotherFuzzTest"},
		},
	}

	// copy test data project to temp dir
	testProject := filepath.Join("testdata", "jazzer", "maven")
	projectDir := shared.CopyCustomTestdataDir(t, testProject, "maven")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })

	// create temp for bundle output
	tempDir := testutil.MkdirTemp(t, "", "cifuzz-archive-*")
	bundlePath := filepath.Join(tempDir, "fuzz_tests.tar.gz")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase %d", i), func(t *testing.T) {
			args := []string{
				"bundle",
				"-o", bundlePath,
			}
			args = append(args, tc.fuzzTargets...)
			metadata, _ := shared.TestRunBundle(t, projectDir, cifuzz, bundlePath, os.Environ(), args...)

			// collect fuzz tests from metadata
			var fuzzTests []string
			for _, fuzzer := range metadata.Fuzzers {
				fuzzTests = append(fuzzTests, fuzzer.Name)
			}

			assert.ElementsMatch(t, tc.expectedFuzzTests, fuzzTests)
		})
	}
}

func TestGetUniqueArtifactName(t *testing.T) {
	basePath := filepath.Join("testdata", "jazzer", "project", "lib")

	testCases := []struct {
		dependency         string
		uniqueArtifactName string
	}{
		{
			dependency:         filepath.Join(basePath, "mylib.jar"),
			uniqueArtifactName: "mylib.jar",
		},
		{
			dependency:         filepath.Join(basePath, "other", "mylib.jar"),
			uniqueArtifactName: "mylib-1.jar",
		},
		{
			dependency:         filepath.Join(basePath, "testlib.jar"),
			uniqueArtifactName: "testlib.jar",
		},
	}

	artifactsMap := make(map[string]uint)

	for _, tc := range testCases {
		name := getUniqueArtifactName(tc.dependency, artifactsMap)
		assert.Equal(t, tc.uniqueArtifactName, name)
	}
}
