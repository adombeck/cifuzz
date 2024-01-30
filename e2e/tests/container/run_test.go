package container_test

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e"
)

var containerRunTests = &[]e2e.TestCase{
	{
		Description: "container run command is available in --help output",
		Command:     "container run",
		Args:        []string{"--help"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("container")
		},
	},
	{
		Description:   "container run command in a maven/gradle example folder is available and fuzzer finds an RCE",
		Command:       "container run",
		Args:          []string{"com.example.FuzzTestCase::myFuzzTest"},
		SampleFolder:  []string{"../../../examples/maven", "../../../examples/gradle"},
		ToolsRequired: []string{"docker", "java", "maven"},
		SkipOnOS:      "windows",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().ErrorContains("Remote Code Execution in com.example.ExploreMe.exploreMe")
		},
	},
}

func TestContainerRun(t *testing.T) {
	e2e.RunTests(t, *containerRunTests, nil)
}
