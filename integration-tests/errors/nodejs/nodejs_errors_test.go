package nodejs

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_NodeJSErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testdataTmp := shared.CopyTestdataDir(t, "nodejs")
	t.Cleanup(func() { fileutil.Cleanup(testdataTmp) })

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))
	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath: cifuzz,
	}

	// Execute npm install
	cmd := exec.Command("npm", "install")
	cmd.Dir = testdataTmp
	err := cmd.Run()
	require.NoError(t, err)

	testCases := []struct {
		id       string
		fuzzTest string
	}{
		{
			id:       "os_command_injection",
			fuzzTest: "command-injection",
		},
		{
			id:       "File Path Injection",
			fuzzTest: "path-traversal",
		},
		{
			id:       "prototype_pollution",
			fuzzTest: "prototype-pollution",
		},
		{
			id:       "timeout",
			fuzzTest: "timeout",
		},
		{
			id:       "Crash",
			fuzzTest: "unhandled-exception",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			cifuzzRunner.Run(t, &shared.RunOptions{
				FuzzTest: tc.fuzzTest,
				WorkDir:  testdataTmp,
			})

			// Call findings command, get json output and check for finding id
			_, findingsJSON := cifuzzRunner.CommandOutput(t, "findings", &shared.CommandOptions{
				Args: []string{
					"--json",
					"--interactive=false",
				},
				WorkDir: testdataTmp,
			})

			var findings []finding.Finding
			err := json.Unmarshal([]byte(findingsJSON), &findings)
			require.NoError(t, err)
			idFound := false
			for _, f := range findings {
				if f.MoreDetails.ID == tc.id {
					idFound = true
					break
				}
			}
			assert.True(t, idFound, "id '%s' not found", tc.id)
		})
	}
}
