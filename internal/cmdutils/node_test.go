package cmdutils

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestValidateNodeFuzzTest(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Copy node testdata
	projectDir := shared.CopyCustomTestdataDir(t, filepath.Join("testdata", "node"), "nodejs")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })
	log.Infof("Project dir: %s", projectDir)

	// Install node modules
	cmd := exec.Command("npm", "install")
	cmd.Dir = projectDir
	t.Logf("Command: %s", cmd.String())
	err := cmd.Run()
	require.NoError(t, err)

	// Valid test path pattern and valid test name pattern
	err = ValidateNodeFuzzTest(projectDir, "FuzzTestCase", "My fuzz test")
	require.NoError(t, err)

	// Invalid test path pattern
	err = ValidateNodeFuzzTest(projectDir, "BuzzTestCase", "My fuzz test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No valid fuzz test found")

	// Invalid test name pattern
	err = ValidateNodeFuzzTest(projectDir, "FuzzTestCase", "My buzz test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No valid fuzz test found")

	// Multiple fuzz tests found
	err = ValidateNodeFuzzTest(projectDir, "FuzzTestCase", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Multiple fuzz tests found")
}
