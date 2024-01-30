package sourcemap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSourceMap(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	projectDir := filepath.Join(cwd, "testdata")
	sourceDirs := []string{
		filepath.Join(projectDir, "src", "main", "java"),
	}

	sourceMap, err := CreateSourceMap(projectDir, sourceDirs)
	require.NoError(t, err)
	assert.Equal(t, 2, len(sourceMap.JavaPackages))
	assert.Contains(t, sourceMap.JavaPackages["com.example"], "src/main/java/com/example/Example.java")
	assert.Contains(t, sourceMap.JavaPackages["com.other"], "src/main/java/com/other/Other.java")
}
