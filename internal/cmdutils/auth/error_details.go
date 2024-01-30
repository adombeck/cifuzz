package auth

import (
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
)

func getErrorDetailsAndToken(server string) ([]*finding.ErrorDetails, string, error) {
	token, err := GetValidToken(server)
	if err != nil {
		return nil, "", err
	}
	apiClient := api.NewClient(server)
	errorDetails, err := apiClient.GetErrorDetails(token)
	return errorDetails, token, err
}

// TryGetErrorDetailsAndToken tries to get a valid token for the specified
// server and use that to retrieve the error details from the server.
// It returns the error details and the token if successful.
// If there is no valid token or the server is unreachable, it prints a
// warning. Only unexpected errors are returned.
func TryGetErrorDetailsAndToken(server string) ([]*finding.ErrorDetails, string, error) {
	errorDetails, token, err := getErrorDetailsAndToken(server)

	var connErr *api.ConnectionError
	var apiErr *api.APIError
	var noValidTokenError *NoValidTokenError
	if errors.As(err, &noValidTokenError) {
		// This error is returned by GetValidToken if there is no valid token.
		log.Infof(messaging.UsageWarning())
		log.Warn("Findings are not supplemented with error details from CI Sense")
		return nil, "", nil
	}
	if errors.As(err, &apiErr) {
		// This error is returned by apiClient.GetErrorDetails if the API request
		// fails.
		log.Warnf("Failed to fetch error details: %v\nFindings are not supplemented with error details from CI Sense", apiErr.Error())
		log.Infof("Continuing without access to error details from CI Sense")
		return nil, "", nil
	}
	if errors.As(err, &connErr) {
		// This error is returned if either GetValidToken or apiClient.GetErrorDetails
		// fail to connect to the server.
		log.Warnf("Failed to connect to server: %v\nFindings are not supplemented with error details from CI Sense.", connErr)
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}

	log.Success("You are authenticated.")
	return errorDetails, token, nil
}

// TryGetErrorDetails does the same as TryGetErrorDetailsAndToken, but
// only returns the error details.
func TryGetErrorDetails(server string) ([]*finding.ErrorDetails, error) {
	errorDetails, _, err := TryGetErrorDetailsAndToken(server)
	return errorDetails, err
}
