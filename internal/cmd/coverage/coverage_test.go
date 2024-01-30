package coverage

import (
	"fmt"
	"os"
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

func TestFail(t *testing.T) {
	_, _, err := cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.Error(t, err)
}

func TestClangMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.Clang))

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "coverage-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "my_fuzz_test")
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "clang"))
}

func TestVisualStudioMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.VisualStudio))

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "coverage-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "my_fuzz_test")
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "Visual Studio"))
}

func TestCMakeMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.CMake))

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "coverage-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "my_fuzz_test")
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "cmake"))
}

func TestLlvmCovVersion(t *testing.T) {
	dependencies.TestMockAllDeps(t)

	dep := dependencies.GetDep(dependencies.LLVMCov)
	version := dependencies.OverwriteGetVersionWith0(dep)

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "coverage-cmd-test", config.BuildSystemCMake)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "my_fuzz_test")
	require.Error(t, err)

	assert.Contains(t, stdErr,
		fmt.Sprintf(dependencies.MessageVersion, "llvm-cov", dep.MinVersion.String(), version))
}

func TestNodeMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.Node))

	// clone the example project because this command needs to parse an actual
	// project config... if there is none it will fail before the dependency check
	testutil.BootstrapExampleProjectForTest(t, "coverage-cmd-test", config.BuildSystemNodeJS)

	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "FuzzTestCase")
	require.Error(t, err)

	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "node"))
}
