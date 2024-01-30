package shared

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared/mockserver"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestRunNotAuthenticated(t *testing.T, dir string, cifuzz string, args ...string) {
	// Start a mock server to handle our requests
	server := mockserver.New(t)
	server.Handlers["/v1/projects"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	server.Start(t)

	tempDir := testutil.MkdirTemp(t, "", "cifuzz-run-*")

	// Create a dictionary
	dictPath := filepath.Join(tempDir, "some_dict")
	err := os.WriteFile(dictPath, []byte("test-dictionary-content"), 0o600)
	require.NoError(t, err)

	// Create a seed corpus directory with an empty seed
	seedCorpusDir, err := os.MkdirTemp(tempDir, "seeds-")
	require.NoError(t, err)
	err = fileutil.Touch(filepath.Join(seedCorpusDir, "empty"))
	require.NoError(t, err)

	args = append(
		[]string{
			"run",
			"--interactive=false",
			"--server=" + server.AddressOnHost(),
			"--no-notifications",
			"crashing_fuzz_test",
		}, args...)

	cmd := executil.Command(cifuzz, args...)
	cmd.Dir = dir
	cmd.Env, err = envutil.Setenv(os.Environ(), "CIFUZZ_API_TOKEN", "")
	require.NoError(t, err)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err)

	assert.Contains(t, string(out), "You are not authenticated with CI Sense.")
}
