package container_test

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e"
	"code-intelligence.com/cifuzz/integration-tests/shared/mockserver"
)

var containerRemoteRunTests = &[]e2e.TestCase{
	{
		Description: "container remote-run command is available in --help output",
		Command:     "container remote-run",
		Args:        []string{"--help"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("container")
		},
	},
	{
		Description:   "container remote-run command in a maven/gradle example folder is available and pushes it to a registry",
		CIUser:        e2e.LoggedInCIUser,
		Command:       "container remote-run",
		Args:          []string{" --project test-project --registry localhost:5000/test/cifuzz com.example.FuzzTestCase::myFuzzTest -v"},
		SampleFolder:  []string{"../../../examples/maven", "../../../examples/gradle"},
		ToolsRequired: []string{"docker", "java", "maven"},
		SkipOnOS:      "windows",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().ErrorContains("Created fuzz container image with ID ")
			output.Success().ErrorContains("Start uploading image ")
			output.Success().ErrorContains("to localhost:5000/test/cifuzz")
			output.Success().ErrorContains("The push refers to repository [localhost:5000/test/cifuzz]")
		},
	},
}

func TestContainerRemoteRun(t *testing.T) {
	mockServer := mockserver.New(t)
	mockServer.Handlers["/v1/projects"] = mockserver.ReturnResponse(t, mockserver.ProjectsJSON)
	mockServer.Handlers["/v3/runs"] = mockserver.ReturnResponse(t, mockserver.ContainerRemoteRunResponse)
	e2e.RunTests(t, *containerRemoteRunTests, mockServer)
}
