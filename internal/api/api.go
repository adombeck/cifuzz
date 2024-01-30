package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/cmd/remoterun/progress"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/version"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/stringutil"
)

// APIError is returned when a REST request returns a status code other
// than 200 OK
type APIError struct {
	err        error
	StatusCode int
}

func (e APIError) Error() string {
	return e.err.Error()
}

func (e APIError) Format(s fmt.State, verb rune) {
	if formatter, ok := e.err.(fmt.Formatter); ok {
		formatter.Format(s, verb)
	} else {
		_, _ = io.WriteString(s, e.Error())
	}
}

func (e APIError) Unwrap() error {
	return e.err
}

// responseToAPIError converts a non-200 response to an APIError with the
// response status code and message.
func responseToAPIError(resp *http.Response) error {
	msg := resp.Status
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &APIError{StatusCode: resp.StatusCode, err: errors.New(msg)}
	}
	apiResp := struct {
		Code    int
		Message string
	}{}
	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		return &APIError{StatusCode: resp.StatusCode, err: errors.Errorf("%s: %s", msg, string(body))}
	}
	return &APIError{StatusCode: resp.StatusCode, err: errors.Errorf("%s: %s", msg, apiResp.Message)}
}

// ConnectionError is returned when a REST request fails to connect to the API
type ConnectionError struct {
	err error
}

func (e ConnectionError) Error() string {
	return e.err.Error()
}

func (e ConnectionError) Unwrap() error {
	return e.err
}

// WrapConnectionError wraps an error returned by the API client in a
// ConnectionError to avoid having the error message printed when the error is
// handled.
func WrapConnectionError(err error) error {
	return &ConnectionError{err}
}

type APIClient struct {
	Server    string
	UserAgent string
}

var FeaturedProjectsOrganization = "organizations/1"

type Artifact struct {
	DisplayName  string `json:"display-name"`
	ResourceName string `json:"resource-name"`
}

func NewClient(server string) *APIClient {
	return &APIClient{
		Server:    server,
		UserAgent: "cifuzz/" + version.Version + " " + runtime.GOOS + "-" + runtime.GOARCH,
	}
}

// ConvertProjectNameFromAPI converts a project name from the API format to a
// format we can use internally.
// The API format is projects/<project-name>, where <project-name> is URL encoded.
// The internal format is <project-name>, where <project-name> is URL decoded.
// We want to use the internal format internally because it's more user friendly and readable.
func ConvertProjectNameFromAPI(projectName string) (string, error) {
	projectName = strings.TrimPrefix(projectName, "projects/")

	// unescape the name here so that we don't have to do it everywhere.
	// We only need to escape it when we send it to the API.
	projectName, err := url.QueryUnescape(projectName)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return projectName, nil
}

// ConvertProjectNameForUseWithAPIV1V2 converts a project name from the internal
// format to the API format. The API format is projects/<project-name>, where
// <project-name> is URL encoded.
func ConvertProjectNameForUseWithAPIV1V2(projectName string) string {
	// remove the projects/ prefix if it exists so that we can call PathEscape
	if strings.HasPrefix(projectName, "projects/") {
		projectName = strings.TrimPrefix(projectName, "projects/")
	}
	// escape the name here so that we don't have to do it everywhere.
	// We only need to escape it when we send it to the API.
	projectName = url.PathEscape(projectName)

	// add the projects/ prefix because the API requires it
	projectName = "projects/" + projectName

	return projectName
}

// ConvertProjectNameForUseWithAPIV3 converts a project name to the v3 API
// format. The API format is <project-name>, where <project-name> is URL
// unescaped.
func ConvertProjectNameForUseWithAPIV3(projectName string) (string, error) {
	projectName = strings.TrimPrefix(projectName, "projects/")

	projectName, err := url.QueryUnescape(projectName)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return projectName, nil
}

func (client *APIClient) UploadBundle(path string, projectName string, token string) (*Artifact, error) {

	projectName = ConvertProjectNameForUseWithAPIV1V2(projectName)

	signalHandlerCtx, cancelSignalHandler := context.WithCancel(context.Background())
	routines, routinesCtx := errgroup.WithContext(context.Background())

	// Cancel the routines context when receiving a termination signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	routines.Go(func() error {
		select {
		case <-signalHandlerCtx.Done():
			return nil
		case s := <-sigs:
			log.Warnf("Received %s", s.String())
			return cmdutils.NewSignalError(s.(syscall.Signal))
		}
	})

	// Use a pipe to avoid reading the artifacts into memory at once
	r, w := io.Pipe()
	m := multipart.NewWriter(w)

	// Write the artifacts to the pipe
	routines.Go(func() error {
		defer w.Close()
		defer m.Close()

		part, err := m.CreateFormFile("fuzzing-artifacts", path)
		if err != nil {
			return errors.WithStack(err)
		}

		fileInfo, err := os.Stat(path)
		if err != nil {
			return errors.WithStack(err)
		}

		f, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer f.Close()

		var reader io.Reader
		printProgress := term.IsTerminal(int(os.Stdout.Fd()))
		if printProgress {
			fmt.Println("Uploading...")
			reader = progress.NewReader(f, fileInfo.Size(), "Upload complete")
		} else {
			reader = f
		}

		_, err = io.Copy(part, reader)
		return errors.WithStack(err)
	})

	// Send a POST request with what we read from the pipe. The request
	// gets cancelled with the routines context is cancelled, which
	// happens if an error occurs in the io.Copy above or the user if
	// cancels the operation.
	var body []byte
	routines.Go(func() error {
		var err error
		defer func() {
			closeErr := r.CloseWithError(err)
			if closeErr != nil {
				log.Warnf("Failed to close pipe: %v", closeErr)
			}
		}()
		defer cancelSignalHandler()
		url, err := url.JoinPath(client.Server, "v2", projectName, "artifacts", "import")
		if err != nil {
			return errors.WithStack(err)
		}
		req, err := http.NewRequestWithContext(routinesCtx, "POST", url, r)
		if err != nil {
			return errors.WithStack(err)
		}

		req.Header.Set("User-Agent", client.UserAgent)
		req.Header.Set("Content-Type", m.FormDataContentType())
		req.Header.Add("Authorization", "Bearer "+token)

		httpClient := &http.Client{Transport: getCustomTransport()}
		resp, err := httpClient.Do(req)
		if err != nil {
			return errors.WithStack(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return responseToAPIError(resp)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})

	err := routines.Wait()
	if err != nil {
		// Routines.Wait() returns our own errors so it should already have
		// a stack trace and doesn't need to have one added
		// nolint: wrapcheck
		return nil, err
	}

	artifact := &Artifact{}
	err = json.Unmarshal(body, artifact)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to parse response from upload bundle API call")
	}

	return artifact, nil
}

func (client *APIClient) StartRemoteFuzzingRun(artifact *Artifact, token string) (string, error) {
	url, err := url.JoinPath("/v1", artifact.ResourceName+":run")
	if err != nil {
		return "", errors.WithStack(err)
	}
	resp, err := client.sendRequest("POST", url, nil, token)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", responseToAPIError(resp)
	}

	// Get the campaign run name from the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.WithStack(err)
	}
	var objmap map[string]json.RawMessage
	err = json.Unmarshal(body, &objmap)
	if err != nil {
		return "", errors.WithStack(err)
	}
	campaignRunNameJSON, ok := objmap["name"]
	if !ok {
		return "", errors.Errorf("Server response doesn't include run name: %v", stringutil.PrettyString(objmap))
	}
	var campaignRunName string
	err = json.Unmarshal(campaignRunNameJSON, &campaignRunName)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return campaignRunName, nil
}

// sendRequest sends a request to the API server with a default timeout of 30 seconds.
func (client *APIClient) sendRequest(method string, endpoint string, body []byte, token string) (*http.Response, error) {
	// we use 30 seconds as a conservative timeout for the API server to
	// respond to a request. We might have to revisit this value in the future
	// after the rollout of our API features.
	timeout := 30 * time.Second
	return client.sendRequestWithTimeout(method, endpoint, body, token, timeout)
}

// sendRequestWithTimeout sends a request to the API server with a timeout.
func (client *APIClient) sendRequestWithTimeout(method string, endpoint string, body []byte, token string, timeout time.Duration) (*http.Response, error) {
	url, err := url.JoinPath(client.Server, endpoint)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	req.Header.Set("User-Agent", client.UserAgent)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	log.Debugf("Sending HTTP request: %s %s\n%s", method, endpoint, body)
	httpClient := &http.Client{Transport: getCustomTransport(), Timeout: timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, WrapConnectionError(errors.WithStack(err))
	}

	log.Debugf("Received response for HTTP request: %d %s", resp.StatusCode, endpoint)

	return resp, nil
}

// IsTokenValid checks if the token is valid by querying the API server.
func (client *APIClient) IsTokenValid(token string) (bool, error) {
	if token == "" {
		return false, nil
	}

	// TOOD: Change this to use another check without querying projects
	_, err := client.ListProjects(token)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == 401 {
				log.Warnf("Invalid token: Received 401 Unauthorized from server %s", client.Server)
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func validateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return errors.WithStack(err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Errorf("unsupported protocol scheme %q", u.Scheme)
	}
	return nil
}

func ValidateAndNormalizeServerURL(server string) (string, error) {
	// Check if the server option is a valid URL
	err := validateURL(server)
	if err != nil {
		// See if prefixing https:// makes it a valid URL
		err = validateURL("https://" + server)
		if err != nil {
			return "", errors.WithMessagef(err, "Server '%s' is not a valid URL", server)
		}
		server = "https://" + server
	}

	// normalize server URL by removing trailing slash
	url, err := url.JoinPath(server, "")
	if err != nil {
		return "", errors.Wrapf(err, "Failed to normalize server URL '%s", server)
	}
	url = strings.TrimSuffix(url, "/")

	return url, nil
}

func getCustomTransport() *http.Transport {
	// it is not possible to use the default Proxy Environment because
	// of https://github.com/golang/go/issues/24135
	dialer := proxy.FromEnvironment()
	dialContext := func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialer.Dial(network, address)
		if err != nil {
			// This error is being returned to the http package and we
			// don't know if it could have side effects with an added stack
			// trace
			// nolint: wrapcheck
			return nil, err
		}
		return conn, nil
	}
	return &http.Transport{DialContext: dialContext}
}
