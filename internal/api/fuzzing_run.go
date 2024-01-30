package api

type FuzzingRun struct {
	Name                    string                   `json:"name"`
	DisplayName             string                   `json:"display_name"`
	Status                  string                   `json:"status"`
	Metrics                 []*Metrics               `json:"metrics,omitempty"`
	FuzzerRunConfigurations *FuzzerRunConfigurations `json:"fuzzer_run_configurations"`
	FuzzTargetConfig        *FuzzTargetConfig        `json:"fuzz_target_config"`
}

type Metrics struct {
	Timestamp                string `json:"timestamp"`
	ExecutionsPerSecond      int32  `json:"executions_per_second"`
	Features                 int32  `json:"features"`
	CorpusSize               int32  `json:"corpus_size"`
	SecondsSinceLastCoverage string `json:"seconds_since_last_coverage"`
	TotalExecutions          string `json:"total_executions"`
	Edges                    int32  `json:"edges"`
	SecondsSinceLastEdge     string `json:"seconds_since_last_edge"`
}

type FuzzTargetConfig struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name"`
	*CAPIFuzzTarget      `json:"c_api,omitempty"`
	*JavaAPIFuzzTarget   `json:"java_api,omitempty"`
	*NodeJSAPIFuzzTarget `json:"nodejs_api,omitempty"`
}

type CAPIFuzzTarget struct {
	APIFuzzTarget `json:"api"`
}

type JavaAPIFuzzTarget struct {
	APIFuzzTarget `json:"api"`
}

type NodeJSAPIFuzzTarget struct {
	APIFuzzTarget `json:"api"`
}

type APIFuzzTarget struct {
	RelativePath string `json:"relative_path"`
}

type FuzzerRunConfigurations struct {
	Engine       string `json:"engine"`
	NumberOfJobs int64  `json:"number_of_jobs"`
}
