package finding_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/e2e"
	"code-intelligence.com/cifuzz/integration-tests/shared/mockserver"
)

var findingTests = &[]e2e.TestCase{
	{
		Description:  "finding command in an empty folder prints error saying it is not a cifuzz project",
		Command:      "finding",
		SampleFolder: []string{"empty"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().NoOutput().ErrorContains("not a cifuzz project")
		},
	},
	{
		Description:  "finding command in a project without findings",
		Command:      "finding",
		SampleFolder: []string{"project-with-empty-cifuzz-yaml"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().NoOutput().ErrorContains("This project doesn't have any findings yet")
		},
	},
	{
		Description:  "finding command ran by an unauthorized user in a project with or without finding details prints findings table without severity score",
		Command:      "finding",
		SampleFolder: []string{"project-with-findings", "project-with-findings-and-error-details"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdout, "n/a")
			assert.Contains(t, output.Stdout, "src/explore_me.cpp:18:11")
		},
	},
	{
		Description:  "finding command ran by an authorized user in a project with findings prints findings table with severity score and fuzz test name",
		Command:      "finding",
		Args:         []string{"--interactive=false"},
		SampleFolder: []string{"project-with-findings-and-error-details"},
		CIUser:       e2e.LoggedInCIUser,
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.NotContains(t, output.Stdout, "n/a")
			assert.Contains(t, output.Stdout, "9.0")
			assert.Contains(t, output.Stdout, "heap buffer overflow")
			assert.Contains(t, output.Stdout, "src/explore_me.cpp:18:11")
		},
	},
	{
		Description:  "finding command ran by a user with invalid token in a project with findings prints error saying it failed to authenticate and prints n/a as score",
		Command:      "finding",
		SampleFolder: []string{"project-with-findings"},
		Args:         []string{"--interactive=false"},
		CIUser:       e2e.InvalidTokenCIUser,
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stderr, "Invalid token: Received 401 Unauthorized from server ")
			assert.Contains(t, output.Stderr, "Findings are not supplemented with error details from CI Sense")
			// it should not print the actual score
			assert.Contains(t, output.Stdout, "n/a")
			assert.NotContains(t, output.Stdout, "9.0")
		},
	},
	{
		Description:  "finding command with finding name argument ran by an authorized user in a project with findings print findings table",
		Command:      "finding",
		Args:         []string{"funky_angelfish --interactive=false"},
		SampleFolder: []string{"project-with-findings-and-error-details"},
		CIUser:       e2e.LoggedInCIUser,
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdout, "my_fuzz_test")
			assert.Contains(t, output.Stdout, "heap buffer overflow")
			assert.Contains(t, output.Stdout, "src/explore_me.cpp:18:11")
			assert.Contains(t, output.Stderr, "cifuzz found more extensive information about this finding:")
			assert.Contains(t, output.Stderr, "| Severity Level       | Critical                                                                         |")
			assert.Contains(t, output.Stderr, "| Severity Score       | 9.0                                                                              |")
			assert.Contains(t, output.Stderr, "| ASan Example         | https://github.com/google/sanitizers/wiki/AddressSanitizerExampleHeapOutOfBounds |")
			assert.Contains(t, output.Stderr, "| ASan Example         | https://github.com/google/sanitizers/wiki/AddressSanitizerExampleHeapOutOfBounds |")
			assert.Contains(t, output.Stderr, "| CWE: Overflow writes | https://cwe.mitre.org/data/definitions/787.html                                  |")
			assert.Contains(t, output.Stderr, "| CWE: Overflow reads  | https://cwe.mitre.org/data/definitions/125.html                                  |")
		},
	},
	{
		Description:  "finding command with finding name argument ran by unauthorized user and should not print extensive information",
		Command:      "finding",
		Args:         []string{"funky_angelfish"},
		SampleFolder: []string{"project-with-findings-and-error-details"},
		CIUser:       e2e.AnonymousCIUser,
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.NotContains(t, output.Stdout, "cifuzz found more extensive information about this finding:")
		},
	},
}

func TestFindingList(t *testing.T) {
	// skipping test on Windows because the 'host.docker.internal' address does
	// not work on Windows.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	server := mockserver.New(t)
	server.Handlers["/v1/projects"] = mockserver.ReturnResponseIfValidToken(t, mockserver.ProjectsJSON)
	server.Handlers["/v2/error-details"] = mockserver.ReturnResponseIfValidToken(t, mockserver.ErrorDetailsJSON)

	e2e.RunTests(t, *findingTests, server)
}
