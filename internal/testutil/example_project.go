package testutil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/config"
)

// BootstrapExampleProjectForTest copies the given example project to a temporary folder
// and changes into that directory.
func BootstrapExampleProjectForTest(t *testing.T, prefix, exampleName string) string {
	tempDir := ChdirToTempDir(t, prefix)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}

	basepath := filepath.Dir(thisFile)
	examplePath := filepath.Join(basepath, "..", "..", "examples", exampleName)

	err := copy.Copy(examplePath, tempDir)
	if err != nil {
		panic(fmt.Sprintf("copying %v to %v failed: %+v", examplePath, tempDir, errors.WithStack(err)))
	}

	return tempDir
}

func BootstrapEmptyProject(t *testing.T, prefix string) string {
	// Create an empty directory
	projectDir := MkdirTemp(t, "", prefix)

	// Create an empty config file
	_, err := config.CreateProjectConfig(projectDir, "", "")
	require.NoError(t, err)

	return projectDir
}
