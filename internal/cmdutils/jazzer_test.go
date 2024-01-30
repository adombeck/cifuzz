package cmdutils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/stubs"
)

func TestListJVMFuzzTestsByRegex(t *testing.T) {
	projectDir := testutil.MkdirTemp(t, "", "list-jvm-files")
	testDir := filepath.Join(projectDir, "src", "test")

	// create some java files including one valid fuzz test
	javaDir := filepath.Join(testDir, "java", "com", "example")
	err := os.MkdirAll(javaDir, 0o755)
	require.NoError(t, err)
	err = stubs.Create(filepath.Join(javaDir, "FuzzTestCase1.java"), config.Java)
	require.NoError(t, err)
	_, err = os.Create(filepath.Join(javaDir, "UnitTestCase.java"))
	require.NoError(t, err)
	javaDirToFilter := filepath.Join(testDir, "java", "com", "filter", "me")
	err = os.MkdirAll(javaDirToFilter, 0o755)
	require.NoError(t, err)
	err = stubs.Create(filepath.Join(javaDirToFilter, "FuzzTestCase2.java"), config.Java)
	require.NoError(t, err)

	// create some kotlin files including one valid fuzz test
	kotlinDir := filepath.Join(testDir, "kotlin", "com", "example")
	err = os.MkdirAll(kotlinDir, 0o755)
	require.NoError(t, err)
	err = stubs.Create(filepath.Join(kotlinDir, "FuzzTestCase3.kt"), config.Kotlin)
	require.NoError(t, err)
	_, err = os.Create(filepath.Join(kotlinDir, "UnitTestCase.kt"))
	require.NoError(t, err)

	// create some extra files
	resDir := filepath.Join(testDir, "resources")
	err = os.MkdirAll(resDir, 0o755)
	require.NoError(t, err)
	_, err = os.Create(filepath.Join(resDir, "SomeTestData"))
	require.NoError(t, err)

	// Check result
	testDirs := []string{filepath.Join(projectDir, "src", "test")}
	result, err := ListJVMFuzzTestsByRegex(testDirs, "com.example")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "com.example.FuzzTestCase1::myFuzzTest")
	assert.Contains(t, result, "com.example.FuzzTestCase3::myFuzzTest")

	// Check result without filter
	result, err = ListJVMFuzzTestsByRegex(testDirs, "")
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "com.example.FuzzTestCase1::myFuzzTest")
	assert.Contains(t, result, "com.filter.me.FuzzTestCase2::myFuzzTest")
	assert.Contains(t, result, "com.example.FuzzTestCase3::myFuzzTest")
}

func TestListJVMFuzzTestsByRegex_DoesNotExist(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	testDirs := []string{filepath.Join(tempDir, "src", "test")}
	fuzzTests, err := ListJVMFuzzTestsByRegex(testDirs, "")
	require.NoError(t, err)
	require.Empty(t, fuzzTests)
}

func TestGetTargetMethodsFromJVMFuzzTestFileSingleMethod(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "jazzer-*")

	type target struct {
		targetName string
		code       []byte
	}

	testCases := []target{
		{
			targetName: "fuzzWithoutParameter",
			code: []byte(`
package com.example;

import com.code_intelligence.jazzer.junit.FuzzTest;

class FuzzTest {
    @FuzzTest
    public static void fuzzWithoutParameter(byte[] data) {}
}`),
		},
		{
			targetName: "fuzzWithParameter",
			code: []byte(`
package com.example;

import com.code_intelligence.jazzer.junit.FuzzTest;

class FuzzTest {
    @FuzzTest(maxDuration = "1m")
    public static void fuzzWithParameter(byte[] data) {}
}`),
		},
		{
			targetName: "fuzzerTestOneInput",
			code: []byte(`
package com.example;

import com.code_intelligence.jazzer.junit.FuzzTest;

class FuzzTest {
    public static void fuzzerTestOneInput(byte[] data) {}
}`)}}

	for _, tc := range testCases {
		t.Run(tc.targetName, func(t *testing.T) {
			path := filepath.Join(tempDir, fmt.Sprintf("FuzzTest%s.java", tc.targetName))
			err := os.WriteFile(path, tc.code, 0o644)
			require.NoError(t, err)

			result, err := GetTargetMethodsFromJVMFuzzTestFile(path)
			require.NoError(t, err)
			assert.Equal(t, []string{tc.targetName}, result)
		})
	}
}

func TestGetTargetMethodsFromJVMFuzzTestFileMultipleMethods(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "jazzer-*")

	path := filepath.Join(tempDir, "FuzzTest.java")
	err := os.WriteFile(path, []byte(`
package com.example;

import com.code_intelligence.jazzer.junit.FuzzTest;

class FuzzTest {
    @FuzzTest
    public static void fuzz(byte[] data) {}

	@FuzzTest
	public static void fuzz2(byte[] data) {}

	@FuzzTest(maxDuration = "1m")
	public static void fuzz3(byte[] data) {}

	public static void fuzzerTestOneInput(byte[] data) {}
}
`), 0o644)
	require.NoError(t, err)

	result, err := GetTargetMethodsFromJVMFuzzTestFile(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"fuzz", "fuzz2", "fuzz3", "fuzzerTestOneInput"}, result)
}

func TestGetJazzerSeedCorpus(t *testing.T) {
	seedCorpusDir := JazzerSeedCorpus("com.example.FuzzTestCase", "project-dir")
	expectedSeedCorpusDir := filepath.Join(
		"project-dir", "src", "test", "resources", "com", "example", "FuzzTestCaseInputs",
	)
	assert.Equal(t, expectedSeedCorpusDir, seedCorpusDir)
}
