package api

import (
	"encoding/json"
	"io"
	"net/url"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/finding"
)

type errorDetailsJSON struct {
	VersionSchema int                     `json:"version_schema"`
	ErrorDetails  []*finding.ErrorDetails `json:"error_details"`
}

// GetErrorDetails gets the error details from the API
func (client *APIClient) GetErrorDetails(token string) ([]*finding.ErrorDetails, error) {
	if token == "" {
		panic("GetErrorDetails called with empty token")
	}

	// get it from the API
	url, err := url.JoinPath("v2", "error-details")
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

	var errorDetailsFromJSON errorDetailsJSON
	err = json.Unmarshal(body, &errorDetailsFromJSON)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return errorDetailsFromJSON.ErrorDetails, nil
}
