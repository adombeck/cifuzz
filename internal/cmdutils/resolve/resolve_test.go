package resolve

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/config"
)

func TestResolve(t *testing.T) {
	testDataDir := shared.CopyTestdataDir(t, "resolve")
	revertToTestDataDir := func() {
		err := os.Chdir(testDataDir)
		require.NoError(t, err)
	}

	changeWdToTestData := func(dir string) string {
		err := os.Chdir(filepath.Join(testDataDir, dir))
		require.NoError(t, err)
		pwd, err := os.Getwd()
		require.NoError(t, err)
		return pwd
	}

	t.Run("resolveBazel", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveBazel(t, changeWdToTestData("bazel"))
	})

	t.Run("resolveCmake", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveCMake(t, changeWdToTestData("cmake"))
	})

	t.Run("testResolveMaven", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveMaven(t, changeWdToTestData("maven"))
	})

	t.Run("testResolveGradle", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveGradle(t, changeWdToTestData("gradle"))
	})

	t.Run("testResolveMavenWindowsPaths", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveMavenWindowsPaths(t, changeWdToTestData("maven"))
	})

	t.Run("testResolveGradleWindowsPaths", func(t *testing.T) {
		defer revertToTestDataDir()
		testResolveGradleWindowsPaths(t, changeWdToTestData("gradle"))
	})

	t.Run("testResolveNodeJS", func(t *testing.T) {
		defer revertToTestDataDir()
		pwd := changeWdToTestData("nodejs")
		testResolveNodeJS(t, pwd)
	})
}

func testResolveBazel(t *testing.T, pwd string) {
	fuzzTestName := "//src/fuzz_test_1:fuzz_test_1"

	// relative path
	srcFile := filepath.Join("src", "fuzz_test_1", "fuzz_test.cpp")
	resolved, err := resolve(srcFile, config.BuildSystemBazel, pwd)
	require.NoError(t, err)
	require.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemBazel, pwd)
	require.NoError(t, err)
	require.Equal(t, fuzzTestName, resolved)
}

func testResolveCMake(t *testing.T, pwd string) {
	// fuzz test is declared in CMakeLists in same directory
	fuzzTestName := "fuzz_test_1"

	// relative path
	srcFile := filepath.Join("src", "fuzz_test_1", "fuzz_test.cpp")
	resolved, err := resolve(srcFile, config.BuildSystemCMake, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemCMake, pwd)
	require.NoError(t, err)
	require.Equal(t, fuzzTestName, resolved)

	// fuzz test is declared in CMakeLists in parent directory
	fuzzTestName = "fuzz_test_2"

	// relative path
	srcFile = filepath.Join("src", "fuzz_test_2", "fuzz_test.cpp")
	resolved, err = resolve(srcFile, config.BuildSystemCMake, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemCMake, pwd)
	require.NoError(t, err)
	require.Equal(t, fuzzTestName, resolved)
}

func testResolveMaven(t *testing.T, pwd string) {
	fuzzTestName := "com.example.fuzz_test_1.FuzzTestCase"

	// Java file
	// relative path
	srcFile := filepath.Join("src", "test", "java", "com", "example", "fuzz_test_1", "FuzzTestCase.java")
	resolved, err := resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// Kotlin file
	// relative path
	srcFile = filepath.Join("src", "test", "kotlin", "com", "example", "fuzz_test_1", "FuzzTestCase.kt")
	resolved, err = resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)
}

func testResolveGradle(t *testing.T, pwd string) {
	fuzzTestName := "com.example.fuzz_test_1.FuzzTestCase"

	// Java file
	// relative path
	srcFile := filepath.Join("src", "test", "java", "com", "example", "fuzz_test_1", "FuzzTestCase.java")
	resolved, err := resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// Kotlin file
	// relative path
	srcFile = filepath.Join("src", "test", "kotlin", "com", "example", "fuzz_test_1", "FuzzTestCase.kt")
	resolved, err = resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)
}

func testResolveGradleWindowsPaths(t *testing.T, pwd string) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	fuzzTestName := "com.example.fuzz_test_1.FuzzTestCase"

	srcFile := "src/test/java/com/example/fuzz_test_1/FuzzTestCase.java"
	resolved, err := resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	srcFile = "src\\test\\java\\com\\example\\fuzz_test_1\\FuzzTestCase.java"
	resolved, err = resolve(srcFile, config.BuildSystemGradle, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)
}

func testResolveMavenWindowsPaths(t *testing.T, pwd string) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	fuzzTestName := "com.example.fuzz_test_1.FuzzTestCase"

	srcFile := "src/test/java/com/example/fuzz_test_1/FuzzTestCase.java"
	resolved, err := resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	srcFile = "src\\test\\java\\com\\example\\fuzz_test_1\\FuzzTestCase.java"
	resolved, err = resolve(srcFile, config.BuildSystemMaven, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)
}

func testResolveNodeJS(t *testing.T, pwd string) {
	fuzzTestName := "FuzzTestCase"

	// relative path
	srcFile := filepath.Join("src", "test", "FuzzTestCase.fuzz.js")
	resolved, err := resolve(srcFile, config.BuildSystemNodeJS, pwd)
	assert.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)

	// absolute path
	srcFile = filepath.Join(pwd, srcFile)
	resolved, err = resolve(srcFile, config.BuildSystemNodeJS, pwd)
	require.NoError(t, err)
	assert.Equal(t, fuzzTestName, resolved)
}
