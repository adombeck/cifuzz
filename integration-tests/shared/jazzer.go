package shared

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/executil"
)

func TestAdditionalJazzerParameters(t *testing.T, cifuzz, dir string) {
	t.Helper()

	testCases := map[string]struct {
		cmd *executil.Cmd
		arg string
	}{
		"run": {
			cmd: executil.Command(cifuzz,
				"run",
				"-v",
				"--interactive=false",
				"com.example.FuzzTestCase"),
			arg: "--instrumentation_includes=com.**",
		},
		"coverage": {
			cmd: executil.Command(cifuzz,
				"coverage",
				"-v",
				"--output=report",
				"com.example.FuzzTestCase"),
			arg: "-Djazzer.instrumentation_includes=com.**",
		},
		"bundle": {
			cmd: executil.Command(cifuzz,
				"bundle",
				"-v",
				"com.example.FuzzTestCase"),
			arg: "-Djazzer.instrumentation_includes=com.**",
		},
	}

	for _, tc := range testCases {
		tc.cmd.Dir = dir
		tc.cmd.Args = append(tc.cmd.Args, "--engine-arg="+tc.arg)
		out, err := tc.cmd.CombinedOutput()
		require.NoError(t, err)

		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			if strings.Contains(l, "com.code_intelligence.jazzer.Jazzer") {
				assert.Contains(t, l, tc.arg, "java call does not include jazzer flag")
				break
			}
		}
	}
}
