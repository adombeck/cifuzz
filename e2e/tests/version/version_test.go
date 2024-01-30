package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/e2e"
)

var versionTests = &[]e2e.TestCase{
	{
		Description: "using --version prints the version + os/arch",
		Command:     "",
		Args:        []string{"--version"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Equal(t, "", output.Stderr)
			assert.Contains(t, output.Stdout, "cifuzz version ")
			assert.Contains(t, output.Stdout, "Running on ")
		},
	},
}

func TestVersion(t *testing.T) {
	e2e.RunTests(t, *versionTests, nil)
}
