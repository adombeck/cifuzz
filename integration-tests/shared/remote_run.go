package shared

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared/mockserver"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var projectName = "test-project"

func TestRemoteRun(t *testing.T, dir string, cifuzz string, args ...string) {
	server := startMockServer(t)

	tempDir := testutil.MkdirTemp(t, "", "cifuzz-archive-*")

	// Create a dictionary
	dictPath := filepath.Join(tempDir, "some_dict")
	err := os.WriteFile(dictPath, []byte("test-dictionary-content"), 0o600)
	require.NoError(t, err)

	// Create a seed corpus directory with an empty seed
	seedCorpusDir, err := os.MkdirTemp(tempDir, "seeds-")
	require.NoError(t, err)
	err = fileutil.Touch(filepath.Join(seedCorpusDir, "empty"))
	require.NoError(t, err)

	// Try to start a remote run on our mock server
	args = append(
		[]string{
			"remote-run",
			"--dict", dictPath,
			"--engine-arg", "arg1",
			"--engine-arg", "arg2",
			"--seed-corpus", seedCorpusDir,
			"--timeout", "100m",
			"--project", projectName,
			"--server", server.AddressOnHost(),
		}, args...)
	cmd := executil.Command(cifuzz, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env, err = envutil.Setenv(os.Environ(), "CIFUZZ_API_TOKEN", "test-token")
	require.NoError(t, err)

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	TerminateOnSignal(t, cmd)

	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}

func TestRemoteRunWithAdditionalArgs(t *testing.T, cifuzzRunner *CIFuzzRunner, expectedErrorExp *regexp.Regexp, args ...string) {
	server := startMockServer(t)

	args = append([]string{
		"--project", "test-project",
		"--server", server.AddressOnHost(),
		"--", "--non-existent-flag"},
		args...)

	// Run the command and expect it to fail because we passed it a non-existent flag
	cifuzzRunner.Run(t, &RunOptions{
		Command: []string{"remote-run"},
		Args:    args,
		Env:     []string{"CIFUZZ_API_TOKEN=test-token"},
		ExpectedOutputs: []*regexp.Regexp{
			expectedErrorExp,
		},
		ExpectError: true,
	})
}

func startMockServer(t *testing.T) *mockserver.MockServer {
	artifactsName := "test-artifacts-123"

	server := mockserver.New(t)

	// define handlers
	server.Handlers["/v1/projects"] = mockserver.ReturnResponse(t, mockserver.ProjectsJSON)
	server.Handlers[fmt.Sprintf("/v2/projects/%s/artifacts/import", projectName)] = mockserver.ReturnResponse(t,
		fmt.Sprintf(`{"display-name": "test-artifacts", "resource-name": %q}`, artifactsName),
	)
	server.Handlers[fmt.Sprintf("/v1/%s:run", artifactsName)] = mockserver.ReturnResponse(t, `{"name": "test-campaign-run-123"}`)

	// start the server
	server.Start(t)

	return server
}
