package mockserver

import (
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/stringutil"
)

const ValidToken = "valid-token"
const InvalidToken = "invalid-token"

//go:embed testdata/projects.json
var ProjectsJSON string

//go:embed testdata/error_details.json
var ErrorDetailsJSON string

//go:embed testdata/remote_findings.json
var RemoteFindingsJSON string

//go:embed testdata/container_remote_run.json
var ContainerRemoteRunResponse string

type MockServer struct {
	listener net.Listener
	Handlers map[string]http.HandlerFunc
}

func New(t *testing.T) *MockServer {
	return &MockServer{
		Handlers: map[string]http.HandlerFunc{
			"/": handleDefault(t),
		},
	}
}

// AddressOnHost returns the address of the mock server on the host machine
// that local clients can use to connect to it.
func (server *MockServer) AddressOnHost() string {
	return fmt.Sprintf("http://%s:%d", "127.0.0.1", server.listener.Addr().(*net.TCPAddr).Port)
}

// AddressInContainer returns the address of the mock server as seen from
// within a container.
// Note: While host.docker.internal is supposed to work on all platforms, it
// unfortunately doesn't on Windows. Therefore, we skip tests that require this
// functionality on Windows, e.g., see TestFindingList in e2e.
func (server *MockServer) AddressInContainer() string {
	// If we're running in a container, we need to use the host.docker.internal
	// address, so that the container can access the host machine.
	return fmt.Sprintf("http://%s:%d", "host.docker.internal", server.listener.Addr().(*net.TCPAddr).Port)
}

func (server *MockServer) Start(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode (requires docker)")
	}

	mux := http.NewServeMux()
	for path, handler := range server.Handlers {
		mux.Handle(path, handler)
	}

	var err error
	server.listener, err = net.Listen("tcp4", ":0")
	require.NoError(t, err)

	go func() {
		err = http.Serve(server.listener, mux)
		require.NoError(t, err)
	}()
}

func ReturnResponse(t *testing.T, responseString string) http.HandlerFunc {
	return CheckBodyAndReturnResponse(t, responseString, nil, nil)
}

func ReturnResponseIfValidToken(t *testing.T, responseString string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "Bearer "+ValidToken {
			CheckBodyAndReturnResponse(t, responseString, nil, nil)(w, req)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
}

func CheckBodyAndReturnResponse(t *testing.T, responseString string, expected []string, unexpected []string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		for _, expectedString := range expected {
			require.Contains(t, string(body), expectedString)
		}
		for _, unexpectedString := range unexpected {
			require.NotContains(t, string(body), unexpectedString)
		}
		_, err = io.WriteString(w, responseString)
		require.NoError(t, err)
	}
}

func handleDefault(t *testing.T) http.HandlerFunc {
	return func(_ http.ResponseWriter, req *http.Request) {
		require.Fail(t, "Unexpected request", stringutil.PrettyString(req))
	}
}
