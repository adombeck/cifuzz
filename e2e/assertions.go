package e2e

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/stretchr/testify/require"
)

func (co *CommandOutput) Success() *CommandOutput {
	require.EqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) Failed() *CommandOutput {
	require.NotEqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) OutputContains(expected string) *CommandOutput {
	if !strings.Contains(co.Stdout, expected) {
		require.FailNow(co.t, fmt.Sprintf("stdout does not contain %q", expected))
	}
	return co
}

func (co *CommandOutput) OutputNotContains(expected string) *CommandOutput {
	if strings.Contains(co.Stdout, expected) {
		require.FailNow(co.t, fmt.Sprintf("stdout contains %q", expected))
	}
	return co
}

func (co *CommandOutput) ErrorContains(expected string) *CommandOutput {
	if !strings.Contains(co.Stderr, expected) {
		require.FailNow(co.t, fmt.Sprintf("stderr does not contain %q", expected))
	}
	return co
}

func (co *CommandOutput) NoOutput() *CommandOutput {
	require.Empty(co.t, co.Stdout)
	return co
}

func (co *CommandOutput) NoError() *CommandOutput {
	require.Empty(co.t, co.Stderr)
	return co
}

func (co *CommandOutput) FileExists(path string) *CommandOutput {
	stat, err := fs.Stat(co.Workdir, path)
	require.NotErrorIs(co.t, err, os.ErrNotExist)
	require.False(co.t, stat.IsDir())
	return co
}
