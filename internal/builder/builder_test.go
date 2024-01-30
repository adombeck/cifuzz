package builder

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestIntegration_Version(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	version := "1.2.3-test"
	testDir := testutil.MkdirTemp(t, "", "builder-test-*")
	builder, err := NewCIFuzzBuilder(Options{
		Version:   version,
		TargetDir: testDir,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Coverage:  true,
	})
	require.NoError(t, err)

	err = builder.BuildCIFuzz()
	require.NoError(t, err)

	cifuzz := filepath.Join(testDir, "bin", "cifuzz")
	cmd := exec.Command(cifuzz, "--version")
	_, cmd.Env = testutil.SetupCoverage(t, os.Environ(), "integration")
	require.NoError(t, err)
	out, err := cmd.Output()
	require.NoError(t, err)

	assert.Contains(t, string(out), version)

}
