package init

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)
	m.Run()
}

func TestInitCmd(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "init-cmd-test", config.BuildSystemCMake)

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	_, _, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.NoError(t, err)

	// second execution should return a ErrSilent as the config file should aready exists
	_, _, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cmdutils.ErrSilent)
}

// TestInitCmdForNodeWithJSLanguageArg tests the init command for Node.js projects (JS).
func TestInitCmdForNodeWithJSLanguageArg(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "init-cmd-test", config.BuildSystemNodeJS)

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "js")
	assert.NoError(t, err)
	assert.Contains(t, stdErr, "jest.config.js")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))
}

// TestInitCmdForNodeWithTSLanguageArg tests the init command for Node.js projects (TS).
func TestInitCmdForNodeWithTSLanguageArg(t *testing.T) {
	testDir := testutil.BootstrapExampleProjectForTest(t, "init-cmd-test", config.BuildSystemNodeJS)

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "ts")
	assert.NoError(t, err)
	assert.Contains(t, stdErr, "jest.config.ts")
	assert.Contains(t, stdErr, "To introduce the fuzz function types globally, add the following import to globals.d.ts:")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))
}

func TestSupportedInitTestTypes(t *testing.T) {
	// Test that the supportedInitTestTypesMap and supportedInitTestTypes are in sync.
	initTestTypes := supportedInitTestTypes
	sort.Strings(initTestTypes)

	initTestTypesKeys := maps.Keys(supportedInitTestTypesMap)
	sort.Strings(initTestTypesKeys)

	assert.Equal(t, initTestTypesKeys, initTestTypes)
}
