package create

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/dependencies"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)
	m.Run()
}

func TestOk(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemCMake)

	outputFile := filepath.Join(testDir, "fuzz-test.cpp")
	args := []string{
		"cpp",
		"--output", outputFile,
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkMaven(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemMaven)

	outputFile := filepath.Join(testDir, "FuzzTestCase.java")
	args := []string{
		"java",
		"--output", outputFile,
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkGradle(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemGradle)

	outputFile := filepath.Join(testDir, "FuzzTestCase.java")
	args := []string{
		"java",
		"--output", outputFile,
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkJavaScript(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", "nodejs")

	outputFile := filepath.Join(testDir, "myTest.fuzz.js")
	args := []string{
		"js",
		"--output", outputFile,
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkTypeScript(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", "nodejs-typescript")

	outputFile := filepath.Join(testDir, "myTest.fuzz.ts")
	args := []string{
		"ts",
		"--output", outputFile,
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestInvalidType(t *testing.T) {
	args := []string{
		"foo",
	}
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.Error(t, err)
}

func TestCreateCmd_OutDir(t *testing.T) {
	t.Skip()
}

func TestCMakeMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.CMake))

	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemCMake)
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "cmake"))
}

func TestClangVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dep := dependencies.GetDep(dependencies.Clang)
	version := dependencies.OverwriteGetVersionWith0(dep)

	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemCMake)
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	assert.Contains(t, stdErr,
		fmt.Sprintf(dependencies.MessageVersion, "clang", dep.MinVersion.String(), version))
}

func TestVisualStudioVersion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dep := dependencies.GetDep(dependencies.VisualStudio)
	version := dependencies.OverwriteGetVersionWith0(dep)

	testDir := testutil.BootstrapExampleProjectForTest(t, "create-cmd-test", config.BuildSystemCMake)
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	assert.Contains(t, stdErr,
		fmt.Sprintf(dependencies.MessageVersion, "Visual Studio", dep.MinVersion.String(), version))
}
