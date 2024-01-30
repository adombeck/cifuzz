package maven

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
)

func Test_GetTestDir(t *testing.T) {
	projectDir := shared.CopyTestdataDir(t, "maven")

	testDir, err := GetTestDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectDir, "src", "test", "java"), testDir)

	// adjust pom.xml to include tag <testSourceDirectory>
	newTestDir := "fuzztests"
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		[]string{fmt.Sprintf("<testSourceDirectory>%s</testSourceDirectory>", newTestDir)},
		"\t<build>",
		true,
	)
	testDir, err = GetTestDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectDir, newTestDir), testDir)
}

func Test_GetSourceDir(t *testing.T) {
	projectDir := shared.CopyTestdataDir(t, "maven")

	sourceDir, err := GetSourceDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectDir, "src", "main", "java"), sourceDir)

	// adjust pom.xml to include tag <sourceDirectory>
	newSourceDir := "example"
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		[]string{fmt.Sprintf("<sourceDirectory>%s</sourceDirectory>", newSourceDir)},
		"\t<build>",
		true,
	)
	sourceDir, err = GetSourceDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectDir, newSourceDir), sourceDir)
}
