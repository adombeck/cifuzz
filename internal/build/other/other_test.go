package other

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/mocks"
	"code-intelligence.com/cifuzz/util/envutil"
)

func defaultFinderMock(t *testing.T, repoRoot string) *mocks.RunfilesFinderMock {
	t.Helper()

	finderMock := &mocks.RunfilesFinderMock{}
	finderMock.On("CIFuzzIncludePath").Return(filepath.Join(repoRoot, "include"), nil)
	finderMock.On("DumperPath").Return(filepath.Join("lib", "dumper.o"), nil)
	finderMock.On("ClangPath").Return("clang", nil)
	return finderMock
}

func TestEnvsSetInBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	repoRoot, err := builder.FindProjectDir()
	require.NoError(t, err)

	projectDir := filepath.Join(repoRoot, "internal", "build", "other", "testdata")

	finderMock := defaultFinderMock(t, repoRoot)

	output := bytes.Buffer{}

	// "Building" without coverage
	b, err := NewBuilder(&BuilderOptions{
		ProjectDir:     projectDir,
		BuildCommand:   "env | grep FUZZ",
		RunfilesFinder: finderMock,
		Stdout:         &output,
	})
	require.NoError(t, err)

	cmd := "test"
	cmdutils.CurrentInvocation = &cmdutils.Invocation{Command: cmd}

	fuzzTestName := "my_fuzz_test"
	_, err = b.Build(fuzzTestName)
	require.NoError(t, err)

	// Note: Testing the environment variables explicitly here
	// because changing them would be a breaking change
	assert.Contains(t, output.String(), fmt.Sprintf("%s=%s", "CIFUZZ_BUILD_STEP", "fuzzing"), "CIFUZZ_BUILD_STEP for fuzzing is not set correctly in environment")
	assert.Contains(t, output.String(), fmt.Sprintf("%s=%s", "CIFUZZ_BUILD_LOCATION", fuzzTestName), "CIFUZZ_BUILD_LOCATION is not set correctly in environment")
	assert.Contains(t, output.String(), fmt.Sprintf("%s=%s", "FUZZ_TEST", fuzzTestName), "FUZZ_TEST is not set correctly in environment")
	assert.Contains(t, output.String(), fmt.Sprintf("%s=%s", "CIFUZZ_COMMAND", cmd), "CIFUZZ_COMMAND is not set correctly in environment")

	// "Building" for coverage
	b, err = NewBuilder(&BuilderOptions{
		ProjectDir:     projectDir,
		BuildCommand:   "env | grep FUZZ",
		RunfilesFinder: finderMock,
		Stdout:         &output,
		Sanitizers:     []string{"coverage"},
	})
	require.NoError(t, err)

	_, err = b.Build(fuzzTestName)
	require.NoError(t, err)
	assert.Contains(t, output.String(), fmt.Sprintf("%s=%s", "CIFUZZ_BUILD_STEP", "coverage"), "CIFUZZ_BUILD_STEP for coverage is not set correctly in environment")
}

// regression test for CLI-1128
// environment variables for c/cxx flags should enclosed by single quotes
func TestNoQuotesOnEnv(t *testing.T) {
	repoRoot, err := builder.FindProjectDir()
	require.NoError(t, err)
	finder := defaultFinderMock(t, repoRoot)

	require.NoError(t, err)

	var env []string
	env, err = SetLibFuzzerEnv(env, finder)
	require.NoError(t, err)
	assert.NotContains(t, envutil.Getenv(env, EnvFuzzTestCFlags), "'")
	assert.NotContains(t, envutil.Getenv(env, EnvFuzzTestCXXFlags), "'")

	env, err = SetCoverageEnv(env, finder)
	require.NoError(t, err)
	assert.NotContains(t, envutil.Getenv(env, EnvFuzzTestCFlags), "'")
	assert.NotContains(t, envutil.Getenv(env, EnvFuzzTestCXXFlags), "'")
}
