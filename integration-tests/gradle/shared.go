package gradle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattn/go-zglob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestBundleGradle(t *testing.T, lang string, dir string, cifuzz string, args ...string) {
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
	assert.Equal(t, "my-image", metadata.Docker)

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
	switch lang {
	case "java":
		assert.Equal(t, 18, len(jarMatches))
	case "kotlin":
		assert.Equal(t, 16, len(jarMatches))
	}

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

	var expectedSourceLocations []string
	switch lang {
	case "java":
		expectedSourceLocations = []string{
			"src/test/java/com/example/FuzzTestCase.java",
			"src/main/java/com/example/ExploreMe.java",
			"src/main/java/com/example/App.java",
		}
	case "kotlin":
		expectedSourceLocations = []string{
			"src/test/kotlin/com/example/FuzzTestCase.kt",
			"src/main/kotlin/com/example/ExploreMe.kt",
			"src/main/kotlin/com/example/Main.kt",
		}
	}
	assert.Equal(t, len(expectedSourceLocations), len(sourceMap.JavaPackages["com.example"]))
	for _, expectedSourceLocation := range expectedSourceLocations {
		assert.Contains(t, sourceMap.JavaPackages["com.example"], expectedSourceLocation)
	}
}
