package api

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

type ContainerRun struct {
	Image     string      `json:"image"`
	FuzzTests []*FuzzTest `json:"fuzz_tests,omitempty"`

	// ProjectNid is the new project ID format used in responses from CI Sense.
	// Future responses from the /v3 API will use these nano IDs and are usually
	// prefixed with "prj", "job", "org", etc.
	ProjectNid string `json:"project_nid,omitempty"`

	// ProjectExternalID is the conventional project id used in the URL of CI Sense
	ProjectExternalID string `json:"project_external_id,omitempty"`
}

type ContainerRunResponse struct {
	Run   *Run    `json:"run,omitempty"`
	Links []*Link `json:"links,omitempty"`
}

type FuzzTest struct {
	Name string `json:"name"`
	Jobs []*Job `json:"jobs,omitempty"`
}

type Link struct {
	Href   string `json:"href,omitempty"`
	Rel    string `json:"rel,omitempty"`
	Method string `json:"method,omitempty"`
}

type Run struct {
	Nid       string      `json:"nid,omitempty"`
	Image     string      `json:"image,omitempty"`
	FuzzTests []*FuzzTest `json:"fuzz_tests,omitempty"`
}
type Job struct {
	Nid    string `json:"nid,omitempty"`
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
	Config string `json:"config,omitempty"`
}

// PostContainerRemoteRun posts a new container run to the CI Sense API at /v3/runs.
// project does not need to have a projects/ prefix and needs to be
func (client *APIClient) PostContainerRemoteRun(image string, project string, fuzzTests []string, token string) error {
	tests := []*FuzzTest{}
	for _, fuzzTest := range fuzzTests {
		tests = append(tests, &FuzzTest{Name: fuzzTest})
	}

	// the /v3 project_external_id is the project name without the projects/
	// prefix and url escaped
	project = strings.TrimPrefix(project, "projects/")
	project, err := url.QueryUnescape(project)
	if err != nil {
		return errors.WithStack(err)
	}

	containerRun := &ContainerRun{
		Image:             image,
		FuzzTests:         tests,
		ProjectExternalID: project,
	}

	body, err := json.Marshal(containerRun)
	if err != nil {
		return errors.WithStack(err)
	}

	url, err := url.JoinPath("/v3", "runs")
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := client.sendRequest("POST", url, body, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return responseToAPIError(resp)
	}

	return nil
}