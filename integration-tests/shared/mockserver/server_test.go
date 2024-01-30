package mockserver

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockServer(t *testing.T) {
	t.Parallel()

	server := New(t)
	server.Handlers["/projects"] = ReturnResponse(t, ProjectsJSON)
	server.Handlers["/error-details"] = ReturnResponse(t, ErrorDetailsJSON)
	server.Start(t)

	// test projects endpoint
	req, err := http.NewRequest("GET", server.AddressOnHost()+"/projects", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, ProjectsJSON, string(respBody))

	// test error details endpoint
	req, err = http.NewRequest("GET", server.AddressOnHost()+"/error-details", nil)
	require.NoError(t, err)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, ErrorDetailsJSON, string(respBody))
}

func TestMockServerNotAuthenticated(t *testing.T) {
	server := New(t)
	server.Handlers["/projects"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	server.Start(t)

	// test projects endpoint
	req, err := http.NewRequest("GET", server.AddressOnHost()+"/projects", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	// assert that the request was not authenticated
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "", string(respBody))
}
