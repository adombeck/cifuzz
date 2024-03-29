package integrationtest

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegration_Timeout(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "do_nothing_fuzzer")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "do_nothing_fuzzer", disableMinijail)
		test.Timeout = time.Second
		// Don't limit the number of runs, to ensure that the test stops
		// because of the timeout and not because the runs limit was
		// reached.
		test.RunsLimit = -1

		output, _ := test.Run(t)
		require.Contains(t, output, "DONE")
	})
}
