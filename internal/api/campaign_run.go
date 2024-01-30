package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/pkg/vcs"
)

type CampaignRunBody struct {
	CampaignRun *CampaignRun `json:"campaign_run"`
}

type CampaignRun struct {
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name"`
	Campaign    *Campaign             `json:"campaign"`
	Runs        []*FuzzingRun         `json:"runs"`
	Status      string                `json:"status"`
	Timestamp   string                `json:"timestamp"`
	Revision    *archive.CodeRevision `json:"revision,omitempty"`
}

type Campaign struct {
	MaxRunTime string `json:"max_run_time"`
}

// CreateCampaignRun creates a new campaign run for the given project and
// returns the name of the campaign and fuzzing run. The campaign and fuzzing
// run name is used to identify the campaign run in the API for consecutive
// calls.
func (client *APIClient) CreateCampaignRun(project string, token string, fuzzTarget string, buildSystem string, firstMetrics *report.FuzzingMetric, lastMetrics *report.FuzzingMetric) (string, string, error) {
	fuzzTargetId := base64.URLEncoding.EncodeToString([]byte(fuzzTarget))

	// generate a short random string to use as the campaign run name
	randBytes := make([]byte, 8)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	project = ConvertProjectNameForUseWithAPIV1V2(project)

	fuzzingRunName, err := url.JoinPath(project, "fuzzing_runs", fmt.Sprintf("cifuzz-fuzzing-run-%s", hex.EncodeToString(randBytes)))
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	fuzzTargetConfigName, err := url.JoinPath(project, "fuzz_targets", fuzzTargetId)
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	metricsList := createMetricsForCampaignRun(firstMetrics, lastMetrics)

	apiFuzzTarget := APIFuzzTarget{
		RelativePath: fuzzTarget,
	}
	fuzzTargetConfig := &FuzzTargetConfig{
		Name:        fuzzTargetConfigName,
		DisplayName: fuzzTarget,
	}
	var engine string
	switch buildSystem {
	case config.BuildSystemBazel, config.BuildSystemCMake, config.BuildSystemOther:
		fuzzTargetConfig.CAPIFuzzTarget = &CAPIFuzzTarget{APIFuzzTarget: apiFuzzTarget}
		engine = "LIBFUZZER"
	case config.BuildSystemMaven, config.BuildSystemGradle:
		fuzzTargetConfig.JavaAPIFuzzTarget = &JavaAPIFuzzTarget{APIFuzzTarget: apiFuzzTarget}
		engine = "JAVA_LIBFUZZER"
	case config.BuildSystemNodeJS:
		fuzzTargetConfig.NodeJSAPIFuzzTarget = &NodeJSAPIFuzzTarget{APIFuzzTarget: apiFuzzTarget}
		engine = "JAZZER_JS"
	default:
		return "", "", errors.Errorf("Unsupported build system: %s", buildSystem)
	}

	fuzzingRun := &FuzzingRun{
		Name:        fuzzingRunName,
		DisplayName: "cifuzz-fuzzing-run",
		Status:      "SUCCEEDED",
		FuzzerRunConfigurations: &FuzzerRunConfigurations{
			Engine:       engine,
			NumberOfJobs: 4,
		},
		Metrics:          metricsList,
		FuzzTargetConfig: fuzzTargetConfig,
	}

	campaignRunName, err := url.JoinPath(project, "campaign_runs", fmt.Sprintf("cifuzz-campaign-run-%s", hex.EncodeToString(randBytes)))
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	campaignRunBody := &CampaignRunBody{
		CampaignRun: &CampaignRun{
			Name:        campaignRunName,
			DisplayName: "cifuzz-campaign-run",
			Campaign: &Campaign{
				MaxRunTime: "120s",
			},
			Runs:      []*FuzzingRun{fuzzingRun},
			Status:    "SUCCEEDED",
			Timestamp: time.Now().Format("2006-01-02T15:04:05.999999999Z07:00"),
			Revision:  vcs.CodeRevision(),
		},
	}

	body, err := json.MarshalIndent(campaignRunBody, "", "  ")
	if err != nil {
		return "", "", errors.WithStack(err)
	}

	url, err := url.JoinPath("/v1", project, "campaign_runs")
	if err != nil {
		return "", "", errors.WithStack(err)
	}
	resp, err := client.sendRequest("POST", url, body, token)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", responseToAPIError(resp)
	}

	return campaignRunBody.CampaignRun.Name, fuzzingRun.Name, nil
}

func createMetricsForCampaignRun(firstMetrics *report.FuzzingMetric, lastMetrics *report.FuzzingMetric) []*Metrics {
	// FIXME: We don't have metrics except for the first run. Successive runs
	// will reuse the corpus and inputs from the previous run and thus will not
	// generate new metrics
	var metricsList []*Metrics
	// add metrics if available
	if firstMetrics != nil && lastMetrics != nil {
		var performance int32
		metricsDuration := lastMetrics.Timestamp.Sub(firstMetrics.Timestamp)
		// This if prevents a case where duration is 0 and we divide by 0 a few lines below
		if metricsDuration.Milliseconds() > 0 {
			execs := lastMetrics.TotalExecutions - firstMetrics.TotalExecutions
			performance = int32(float64(execs) / (float64(metricsDuration.Milliseconds()) / 1000))
		} else {
			performance = 0
		}

		metricsList = []*Metrics{
			{
				Timestamp:                lastMetrics.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      performance,
				Features:                 lastMetrics.Features,
				CorpusSize:               lastMetrics.CorpusSize,
				SecondsSinceLastCoverage: fmt.Sprintf("%d", lastMetrics.SecondsSinceLastFeature),
				TotalExecutions:          fmt.Sprintf("%d", lastMetrics.TotalExecutions),
				Edges:                    lastMetrics.Edges,
				SecondsSinceLastEdge:     fmt.Sprintf("%d", lastMetrics.SecondsSinceLastEdge),
			},
			{
				Timestamp:                firstMetrics.Timestamp.Format(time.RFC3339),
				ExecutionsPerSecond:      performance,
				Features:                 firstMetrics.Features,
				CorpusSize:               firstMetrics.CorpusSize,
				SecondsSinceLastCoverage: fmt.Sprintf("%d", firstMetrics.SecondsSinceLastFeature),
				TotalExecutions:          fmt.Sprintf("%d", firstMetrics.TotalExecutions),
				Edges:                    firstMetrics.Edges,
				SecondsSinceLastEdge:     fmt.Sprintf("%d", firstMetrics.SecondsSinceLastEdge),
			},
		}
	}

	return metricsList
}
