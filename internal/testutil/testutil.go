package testutil

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/installer"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var ChdirMutex sync.Mutex

// RegisterTestDeps ensures that the test calling this function is rerun (despite caching) if any of the files and
// directories (and their recursive contents) under the provided paths change.
func RegisterTestDeps(path ...string) {
	// Workaround for https://github.com/golang/go/issues/53053
	// Explicitly stat all data dirs and files so that the Go test runner picks up the data dependency and knows how to
	// rerun the test if the data dir contents change. Without this explicit recursive walk, changes to files in
	// subdirectories aren't picked up automatically.
	for _, p := range path {
		err := filepath.Walk(p, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}
			_, err = os.Stat(path)
			return errors.WithStack(err)
		})
		if err != nil {
			panic(err)
		}
	}
}

// RegisterTestDepOnCIFuzz registers test dependencies on the cifuzz
// executable and all its dependencies. Go doesn't recognize those
// dependencies on its own in tests which build and execute cifuzz as an
// external command.
func RegisterTestDepOnCIFuzz() {
	var deps []string
	_, b, _, _ := runtime.Caller(0)
	// Note: The number of levels we go up here has to be adjusted if
	// this source file is moved.
	basepath := filepath.Dir(filepath.Dir(filepath.Dir(b)))
	for _, dep := range installer.Deps {
		deps = append(deps, filepath.Join(basepath, dep))
	}
	RegisterTestDeps(deps...)
}

// ChdirToTempDir creates and changes the working directory to new tmp dir
func ChdirToTempDir(t *testing.T, prefix string) string {
	ChdirMutex.Lock()
	oldWd, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get current working directory: %+v", err)
		os.Exit(1)
	}

	testTempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		log.Printf("Failed to create temp dir for tests: %+v", err)
		os.Exit(1)
	}

	err = os.Chdir(testTempDir)
	if err != nil {
		log.Printf("Failed to change working dir for tests: %+v", err)
		fileutil.Cleanup(testTempDir)
		os.Exit(1)
	}

	t.Cleanup(func() {
		err = os.Chdir(oldWd)
		if err != nil {
			log.Printf("Failed to change working directory back to %s: %+v", oldWd, err)
			os.Exit(1)
		}
		ChdirMutex.Unlock()
		fileutil.Cleanup(testTempDir)
	})

	return testTempDir
}

// CheckOutput checks that the strings are contained in the reader output
func CheckOutput(t *testing.T, r io.Reader, s ...string) {
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	for _, str := range s {
		require.Contains(t, string(output), str)
	}
}

// MkdirTemp wraps os.MkdirTemp and makes sure that errors are checked and
// directories will be deleted
func MkdirTemp(t *testing.T, dir, pattern string) string {
	tempDir, err := os.MkdirTemp(dir, pattern)
	require.NoError(t, err)
	t.Cleanup(func() { fileutil.Cleanup(tempDir) })
	return tempDir
}

// RepoRoot returns the path pointing to the root of the cifuzz project
func RepoRoot(t *testing.T) string {
	_, b, _, _ := runtime.Caller(0)
	// Note: The number of levels we go up here has to be adjusted if
	// this source file is moved.
	basepath := filepath.Dir(filepath.Dir(filepath.Dir(b)))
	return basepath
}

// SetupCoverage creates a directory for coverage data and sets the
// needed environment variable
func SetupCoverage(t *testing.T, env []string, subdir string) (string, []string) {
	t.Helper()
	covDir := filepath.Join(RepoRoot(t), "coverage", subdir)
	err := os.MkdirAll(covDir, 0755)
	require.NoError(t, err)
	env, err = envutil.Setenv(env, "GOCOVERDIR", covDir)
	require.NoError(t, err)
	return covDir, env
}
