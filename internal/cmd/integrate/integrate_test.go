package integrate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	initCmd "code-intelligence.com/cifuzz/internal/cmd/init"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/mocks"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

func TestMissingCIFuzzProject(t *testing.T) {
	// Create an empty project directory and change working directory to it
	testDir := testutil.ChdirToTempDir(t, "integrate-cmd-test")

	// Check that the command produces the expected error when not
	// called below a cifuzz project directory.
	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "git")
	require.Error(t, err)
	assert.Contains(t, stdErr, "cifuzz.yaml file does not exist")

	// Initialize a cifuzz project
	err = fileutil.Touch(filepath.Join(testDir, "CMakeLists.txt"))
	require.NoError(t, err)
	_, _, err = cmdutils.ExecuteCommand(t, initCmd.New(), os.Stdin)
	require.NoError(t, err)

	// Check that command produces no error when called below a cifuzz
	// project directory
	_, _, err = cmdutils.ExecuteCommand(t, New(), os.Stdin, "git")
	require.NoError(t, err)
}

func TestSetupGitIgnore(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "integrate-cmd-test", config.BuildSystemCMake)

	gitIgnorePath := filepath.Join(testDir, ".gitignore")
	cmakeListsPath := filepath.Join(testDir, "CMakeLists.txt")
	// Remove existing .gitignore and CMakeLists.txt from example project
	os.Remove(gitIgnorePath)
	os.Remove(cmakeListsPath)

	// Check that a new .gitignore file is created
	err := setupGitIgnore(testDir)
	require.NoError(t, err)
	content, err := os.ReadFile(gitIgnorePath)
	require.NoError(t, err)
	assert.Equal(t, 2, len(getNonEmptyLines(content)))

	// Check that only nonexistent entries are added
	fileToIgnore := "/.cifuzz-corpus/\n"
	err = os.WriteFile(gitIgnorePath, []byte(fileToIgnore), 0644)
	require.NoError(t, err)
	err = setupGitIgnore(testDir)
	require.NoError(t, err)
	content, err = os.ReadFile(gitIgnorePath)
	require.NoError(t, err)
	assert.Equal(t, 2, len(getNonEmptyLines(content)))

	// Check that two additional entries are added for cmake projects
	err = fileutil.Touch(cmakeListsPath)
	require.NoError(t, err)
	err = setupGitIgnore(testDir)
	require.NoError(t, err)
	content, err = os.ReadFile(gitIgnorePath)
	require.NoError(t, err)
	assert.Equal(t, 4, len(getNonEmptyLines(content)))
}

func TestSetupCMakePresets(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "integrate-cmd-test", config.BuildSystemCMake)

	sourceDir := getRootSourceDirectory(t, testDir)
	cmakePresets := filepath.Join(sourceDir, "share", "integration", "CMakePresets.json")
	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePresetsPath").Return(cmakePresets, nil)

	err := setupCMakePresets(testDir, finder)
	require.NoError(t, err)

	// Check that CMakeUserPresets.json has been created
	presetsPath := filepath.Join(testDir, "CMakeUserPresets.json")
	presetsExists, err := fileutil.Exists(presetsPath)
	require.NoError(t, err)
	require.True(t, presetsExists)

	logOutput := new(bytes.Buffer)
	log.Output = logOutput
	err = setupCMakePresets(testDir, finder)
	require.NoError(t, err)

	// Check that presets are logged if CMakeUserPresets.json already exists
	content, err := os.ReadFile(cmakePresets)
	require.NoError(t, err)
	testutil.CheckOutput(t, logOutput, string(content))
}

func TestSetupVSCodeTasks(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "integrate-cmd-test", config.BuildSystemCMake)

	sourceDir := getRootSourceDirectory(t, testDir)
	vscodeTasks := filepath.Join(sourceDir, "share", "integration", "tasks.json")
	finder := &mocks.RunfilesFinderMock{}
	finder.On("VSCodeTasksPath").Return(vscodeTasks, nil)

	err := setupVSCodeTasks(testDir, finder)
	require.NoError(t, err)

	// Check that tasks.json has been created
	presetsPath := filepath.Join(testDir, ".vscode", "tasks.json")
	presetsExists, err := fileutil.Exists(presetsPath)
	require.NoError(t, err)
	require.True(t, presetsExists)

	logOutput := new(bytes.Buffer)
	log.Output = logOutput
	err = setupVSCodeTasks(testDir, finder)
	require.NoError(t, err)

	// Check that tasks are logged if tasks.json already exists
	content, err := os.ReadFile(vscodeTasks)
	require.NoError(t, err)
	testutil.CheckOutput(t, logOutput, strings.TrimSpace(string(content)))
}

func getNonEmptyLines(content []byte) []string {
	return stringutil.NonEmpty(strings.Split(string(content), "\n"))
}

func getRootSourceDirectory(t *testing.T, testDir string) string {
	// Change to source file directory
	_, file, _, _ := runtime.Caller(0)
	err := os.Chdir(filepath.Dir(file))
	require.NoError(t, err)

	// Find root source directory
	sourceDir, err := builderPkg.FindProjectDir()
	require.NoError(t, err)

	// Change back to test directory
	err = os.Chdir(testDir)
	require.NoError(t, err)

	return sourceDir
}
