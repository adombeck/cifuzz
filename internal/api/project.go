package api

import (
	"encoding/json"
	"io"
	"net/url"

	"github.com/pkg/errors"
)

type ProjectBody struct {
	Project Project `json:"project"`
}

type Project struct {
	Name                  string `json:"name"`
	DisplayName           string `json:"display_name"`
	OwnerOrganizationName string `json:"owner_organization_name,omitempty"`
}

type ProjectResponse struct {
	Name     string    `json:"name"`
	Done     bool      `json:"done"`
	Response *Response `json:"response"`
}

type Response struct {
	Type          string    `json:"@type"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	Location      *Location `json:"location"`
	OwnerUsername string    `json:"owner_username"`
}

type Location struct {
	GitPath *GitPath `json:"git_path"`
}

type GitPath struct{}

func (client *APIClient) ListProjects(token string) ([]*Project, error) {
	url, err := url.JoinPath("/v1", "projects")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := client.sendRequest("GET", url, nil, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, responseToAPIError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var objmap map[string]json.RawMessage
	err = json.Unmarshal(body, &objmap)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var projects []*Project
	// If the projects field is not present, it means there are no projects
	// so we return an empty list of projects and no error.
	if _, ok := objmap["projects"]; !ok {
		return []*Project{}, nil
	}
	err = json.Unmarshal(objmap["projects"], &projects)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Filter out featured projects
	var filteredProjects []*Project
	for _, p := range projects {
		if p.OwnerOrganizationName == FeaturedProjectsOrganization {
			continue
		}

		p.Name, err = ConvertProjectNameFromAPI(p.Name)
		if err != nil {
			return nil, err
		}
		filteredProjects = append(filteredProjects, p)
	}

	return filteredProjects, nil
}

func (client *APIClient) CreateProject(name string, token string) (*Project, error) {
	projectBody := &ProjectBody{
		Project: Project{
			DisplayName: name,
		},
	}

	body, err := json.MarshalIndent(projectBody, "", "  ")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	url, err := url.JoinPath("/v1", "projects")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp, err := client.sendRequest("POST", url, body, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, responseToAPIError(resp)
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var projectResponse ProjectResponse
	err = json.Unmarshal(body, &projectResponse)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	projectBody.Project.Name = projectResponse.Response.Name

	return &projectBody.Project, nil
}
