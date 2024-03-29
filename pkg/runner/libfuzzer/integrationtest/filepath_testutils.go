package integrationtest

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/require"
)

var baseTempDir string
var testDataDir string
var createTestDataDirOnce sync.Once

func TestDataDir(t *testing.T) string {
	createTestDataDirOnce.Do(func() {
		var err error
		_, filename, _, ok := runtime.Caller(0)
		require.True(t, ok, "unable to get filename from runtime")
		srcDir := filepath.Join(filepath.Dir(filename), "testdata")
		testDataDir, err = os.MkdirTemp(baseTempDir, "testdata-")
		require.NoError(t, err)
		opts := copy.Options{
			// The testdata directory contains a symlink to our dumper
			// source directory, which we need to create a hard copy of
			// because the relative symlink would be broken when copied
			OnSymlink: func(string) copy.SymlinkAction {
				return copy.Deep
			},
		}
		err = copy.Copy(srcDir, testDataDir, opts)
		require.NoError(t, err)

		// chdir into the temporary test data dir to keep the current
		// working directory clean
		err = os.Chdir(testDataDir)
		require.NoError(t, err)
	})
	return testDataDir
}

func TempBuildDir(t *testing.T) string {
	buildDir, err := os.MkdirTemp(baseTempDir, "build-")
	require.NoError(t, err)
	return buildDir
}

func FuzzTestExecutablePath(t *testing.T, buildDir, fuzzTest string) string {
	if runtime.GOOS == "windows" {
		fuzzTest += ".exe"
	}
	return filepath.Join(buildDir, fuzzTest)
}
