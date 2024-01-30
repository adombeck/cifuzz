package init_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/e2e"
)

var initTests = &[]e2e.TestCase{
	{
		Description:  "init command in empty CMake project succeeds and creates a config file",
		Command:      "init",
		SampleFolder: []string{"cmake"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
		},
	},
	{
		Description:  "init command with a 'maven' argument should create a config file for java",
		Command:      "init",
		Args:         []string{"maven"},
		SampleFolder: []string{"node-typescript", "nodejs", "cmake", "empty"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
			assert.Contains(t, output.Stdall, "<artifactId>jazzer-junit</artifactId>")
			assert.NotContains(t, output.Stdall, "Failed to create config")
			output.FileExists("cifuzz.yaml")
		},
	},
}

var nodeInitTests = &[]e2e.TestCase{
	{
		Description:  "init command in Node.js (JS) project succeeds and creates a config file",
		Command:      "init",
		Args:         []string{"js"},
		SampleFolder: []string{"nodejs"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "To use jazzer.js, add a dev-dependency to @jazzer.js/jest-runner")
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
			assert.NotContains(t, output.Stdall, "Failed to create config")
			output.FileExists("cifuzz.yaml")
		},
	},
	{
		Description:  "init command in Node.js (TS) project succeeds and creates a config file",
		Command:      "init",
		Args:         []string{"ts"},
		SampleFolder: []string{"node-typescript"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "To use jazzer.js, add a dev-dependency to @jazzer.js/jest-runner")
			assert.Contains(t, output.Stdall, "'jest.config.ts'")
			assert.Contains(t, output.Stdall, "To introduce the fuzz function types globally, add the following import to globals.d.ts:")
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
			assert.NotContains(t, output.Stdall, "Failed to create config")
			output.FileExists("cifuzz.yaml")
		},
	},
}

func TestInit(t *testing.T) {
	e2e.RunTests(t, *initTests, nil)
}

func TestInitForNodejs(t *testing.T) {
	e2e.RunTests(t, *nodeInitTests, nil)
}
