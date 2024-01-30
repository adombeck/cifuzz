package container_test

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e"
)

var containerTests = &[]e2e.TestCase{
	{
		Description: "container command is available in --help output",
		Command:     "--help",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("container")
		},
	},
	{
		Description:  "container command in a project folder is available",
		Command:      "container",
		SampleFolder: []string{"project-with-empty-cifuzz-yaml"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("cifuzz container")
		},
	},
}

func TestContainer(t *testing.T) {
	e2e.RunTests(t, *containerTests, nil)
}
