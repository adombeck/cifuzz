package api

import (
	"encoding/json"
	"io"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/finding"
)

type Findings struct {
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Name                  string       `json:"name"`
	DisplayName           string       `json:"display_name"`
	FuzzTarget            string       `json:"fuzz_target"`
	FuzzingRun            string       `json:"fuzzing_run"`
	CampaignRun           string       `json:"campaign_run"`
	ErrorReport           *ErrorReport `json:"error_report"`
	Timestamp             string       `json:"timestamp"`
	FuzzTargetDisplayName string       `json:"fuzz_target_display_name,omitempty"`
}

type ErrorReport struct {
	Logs      []string `json:"logs"`
	Details   string   `json:"details"`
	Type      string   `json:"type,omitempty"`
	InputData []byte   `json:"input_data,omitempty"`

	DebuggingInfo      *DebuggingInfo        `json:"debugging_info,omitempty"`
	HumanReadableInput string                `json:"human_readable_input,omitempty"`
	MoreDetails        *finding.ErrorDetails `json:"more_details,omitempty"`
	Tag                string                `json:"tag,omitempty"`
	ShortDescription   string                `json:"short_description,omitempty"`
}

type DebuggingInfo struct {
	ExecutablePath string         `json:"executable_path,omitempty"`
	RunArguments   []string       `json:"run_arguments,omitempty"`
	BreakPoints    []*BreakPoint  `json:"break_points,omitempty"`
	Environment    []*Environment `json:"environment,omitempty"`
}

type BreakPoint struct {
	SourceFilePath string           `json:"source_file_path,omitempty"`
	Location       *FindingLocation `json:"location,omitempty"`
	Function       string           `json:"function,omitempty"`
}

type FindingLocation struct {
	Line   uint32 `json:"line,omitempty"`
	Column uint32 `json:"column,omitempty"`
}

type Environment struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Severity struct {
	Description string  `json:"description,omitempty"`
	Score       float32 `json:"score,omitempty"`
}

// DownloadRemoteFindings downloads all remote findings for a given project from CI Sense.
func (client *APIClient) DownloadRemoteFindings(project string, token string) (Findings, error) {
	project = ConvertProjectNameForUseWithAPIV1V2(project)

	remoteFindings := Findings{}

	url, err := url.JoinPath("v1", project, "findings")
	if err != nil {
		return remoteFindings, errors.WithStack(err)
	}

	// setting a timeout of 5 seconds for the request, since we don't want to
	// wait too long, especially when we need to await this request for command
	// completion
	resp, err := client.sendRequestWithTimeout("GET", url, nil, token, 5*time.Second)
	if err != nil {
		return remoteFindings, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return remoteFindings, responseToAPIError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return remoteFindings, errors.WithStack(err)
	}

	err = json.Unmarshal(body, &remoteFindings)
	if err != nil {
		return remoteFindings, errors.WithStack(err)
	}

	return remoteFindings, nil
}

func (client *APIClient) UploadFinding(project string, fuzzTarget string, campaignRunName string, fuzzingRunName string, finding *finding.Finding, token string) error {
	project = ConvertProjectNameForUseWithAPIV1V2(project)

	// loop through the stack trace and create a list of breakpoints
	breakPoints := []*BreakPoint{}
	for _, stackFrame := range finding.StackTrace {
		breakPoints = append(breakPoints, &BreakPoint{
			SourceFilePath: stackFrame.SourceFile,
			Location: &FindingLocation{
				Line:   stackFrame.Line,
				Column: stackFrame.Column,
			},
			Function: stackFrame.Function,
		})
	}

	findings := &Findings{
		Findings: []Finding{
			{
				Name:        project + finding.Name,
				DisplayName: finding.Name,
				FuzzTarget:  fuzzTarget,
				FuzzingRun:  fuzzingRunName,
				CampaignRun: campaignRunName,
				ErrorReport: &ErrorReport{
					Logs:      finding.Logs,
					Details:   finding.Details,
					Type:      string(finding.Type),
					InputData: finding.InputData,
					DebuggingInfo: &DebuggingInfo{
						BreakPoints: breakPoints,
					},
					MoreDetails:      finding.MoreDetails,
					Tag:              finding.Tag,
					ShortDescription: finding.ShortDescriptionColumns()[0],
				},
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
	}

	body, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	url, err := url.JoinPath("/v1", project, "findings")
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
