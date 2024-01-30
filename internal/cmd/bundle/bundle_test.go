package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)

	// Make the bundle command not fail on unsupported platforms to be
	// able to test it on all platforms, reset the env variable after the test.
	allowUnsupportedPlatformsEnv := os.Getenv(config.AllowUnsupportedPlatformsEnv)
	defer func() {
		err := os.Setenv(config.AllowUnsupportedPlatformsEnv, allowUnsupportedPlatformsEnv)
		if err != nil {
			panic(err)
		}
	}()
	err := os.Setenv(config.AllowUnsupportedPlatformsEnv, "1")
	if err != nil {
		panic(err)
	}

	m.Run()
}

func TestUnknownBuildSystem(t *testing.T) {
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin)
	require.Error(t, err)

	// In this scenario a log with the error message will be created and we do not care about it here
	fileutil.Cleanup(".cifuzz-build")
}

func TestClangMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.Clang))

	opts := &options{}
	opts.BuildSystem = config.BuildSystemCMake

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "run-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "clang"))
}

func TestClangVersion(t *testing.T) {
	dependencies.TestMockAllDeps(t)

	dep := dependencies.GetDep(dependencies.Clang)
	version := dependencies.OverwriteGetVersionWith0(dep)

	opts := &options{}
	opts.BuildSystem = config.BuildSystemCMake

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "run-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)

	assert.Contains(t, stdErr,
		fmt.Sprintf(dependencies.MessageVersion, "clang", dep.MinVersion.String(), version))
}

func TestCMakeMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.CMake))

	opts := &options{}
	opts.BuildSystem = config.BuildSystemCMake

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "run-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "cmake"))
}

func TestEnvVarsSetInConfigFile(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "bundle-test-")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })
	configFileContent := `env:
  - FOO=foo
  - BAR
  - NO_SUCH_VARIABLE
`
	err := os.WriteFile(filepath.Join(projectDir, "cifuzz.yaml"), []byte(configFileContent), 0644)
	require.NoError(t, err)

	t.Setenv("BAR", "bar")

	opts := &options{bundler.Opts{
		ProjectDir:  projectDir,
		ConfigDir:   projectDir,
		BuildSystem: config.BuildSystemCMake,
	}}

	cmd := newWithOptions(opts)
	err = cmd.PreRunE(cmd, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"FOO=foo", "BAR=bar"}, opts.Env)
}
