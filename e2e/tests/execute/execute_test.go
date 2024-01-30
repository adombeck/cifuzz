package execute_test

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/e2e"
)

var executeTests = &[]e2e.TestCase{
	{
		Description: "execute command is available in --help output",
		Command:     "--help",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("execute")
		},
	},
	{
		Description:  "execute command in a folder with bundle contents is available and prints a helpful message",
		Command:      "execute",
		SampleFolder: []string{"folder-with-unpacked-bundle"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("This container is based on:")
			output.Success().OutputContains("Available fuzzers:")
		},
	},
	{
		Description:  "execute command with an invalid fuzz test argument in a folder with bundle contents fails",
		Command:      "execute",
		Args:         []string{"invalid.name"},
		SampleFolder: []string{"folder-with-unpacked-bundle"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().ErrorContains("fuzzer 'invalid.name' not found in a bundle metadata file")
		},
	},
	{
		Description:  "execute command with --json-output-file flag to create a file containing the json output",
		Command:      "execute",
		Args:         []string{"com.example.FuzzTestCase --json-output-file test.json"},
		SampleFolder: []string{"folder-with-unpacked-bundle"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.FileExists("test.json")
		},
	},
}

func TestExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("The execute command is not supported on Windows")
	}
	e2e.RunTests(t, *executeTests, nil)
}
