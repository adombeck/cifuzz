package finding

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared/mockserver"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/stringutil"
)

func TestFindingCmd_FailsIfNoCIFuzzProject(t *testing.T) {
	// Create an empty directory
	projectDir := testutil.MkdirTemp(t, "", "test-findings-cmd-fails-")

	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command produces the expected error when not
	// called below a cifuzz project directory.
	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	assert.Contains(t, stdErr, "Failed to parse cifuzz.yaml")
}

func TestListFindings(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command lists no findings in the empty project
	stdOut, _, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, "--json", "--interactive=false")
	require.NoError(t, err)
	require.Equal(t, "[]", stdOut)

	// Create a finding
	f := &finding.Finding{
		Name:   "test_finding",
		Origin: "Local",
	}

	// Add some MoreDetails to check for possible nil pointer errors
	f.MoreDetails = &finding.ErrorDetails{
		ID: "test_id",
	}

	err = f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command lists the finding
	stdOut, _, err = cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, "--json", "--interactive=false")
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString([]*finding.Finding{f})
	require.NoError(t, err)
	require.Equal(t, jsonString, stdOut)
}

func TestListFindings_Authenticated(t *testing.T) {
	t.Setenv("CIFUZZ_API_TOKEN", "token")
	server := mockserver.New(t)
	server.Handlers["/v1/projects"] = mockserver.ReturnResponse(t, mockserver.ProjectsJSON)
	server.Handlers["/v2/error-details"] = mockserver.ReturnResponse(t, mockserver.ErrorDetailsJSON)
	server.Start(t)

	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Create the expected finding
	f := &finding.Finding{
		Origin:  "Local",
		Name:    "test_finding",
		Type:    "undefined behavior",
		Details: "test_details",
		MoreDetails: &finding.ErrorDetails{
			ID:          "undefined_behavior",
			Name:        "Undefined Behavior",
			Description: "An operation has been detected which is undefined by the C/C++ standard. The result will \nbe compiler dependent and is often unpredictable.",
			Severity: &finding.Severity{
				Score: 2.0,
				Level: "Low",
			},
			Mitigation: "Avoid all operations that cause undefined behavior as per the C/C++ standard.",
			Links: []finding.Link{
				{
					Description: "Undefined Behavior Sanitizer",
					URL:         "https://clang.llvm.org/docs/UndefinedBehaviorSanitizer.html#available-checks",
				},
			},
		},
		FuzzTest: "my_fuzz_test",
	}

	err := f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command lists the finding
	stdOut, _, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, "--json", "--server", server.AddressOnHost())
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString([]*finding.Finding{f})
	require.NoError(t, err)
	require.Equal(t, jsonString, stdOut)
}

func TestPrintFinding(t *testing.T) {
	// Create a finding
	f := &finding.Finding{
		Origin: "Local",
		Name:   "test_finding",
	}

	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command produces the expected error when the
	// specified finding does not exist
	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json", "--interactive=false")
	require.Error(t, err)
	assert.Contains(t, stdErr, fmt.Sprintf("Finding %s does not exist", f.Name))

	// Create the finding
	err = f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command prints the finding
	stdOut, _, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json", "--interactive=false")
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString(f)
	require.NoError(t, err)
	require.Equal(t, jsonString, stdOut)

	// Check that the command does not print extra information
	_, stdErr, err = cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--interactive=false")
	require.NoError(t, err)
	require.NotContains(t, stdErr, "cifuzz found more extensive information about this finding:")
}

func TestPrintUsageWarning(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.NoError(t, err)
	require.Contains(t, stdErr, "You are not authenticated with CI Sense.")
}

func TestPrintFinding_Authenticated(t *testing.T) {
	t.Setenv("CIFUZZ_API_TOKEN", "token")
	server := mockserver.New(t)
	server.Handlers["/v1/projects"] = mockserver.ReturnResponse(t, mockserver.ProjectsJSON)
	server.Handlers["/v2/error-details"] = mockserver.ReturnResponse(t, mockserver.ErrorDetailsJSON)
	server.Start(t)

	projectDir := testutil.BootstrapEmptyProject(t, "test-print-finding-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Create the expected finding
	f := &finding.Finding{
		Origin:  "Local",
		Name:    "test_finding",
		Type:    "undefined behavior",
		Details: "test_details",
		MoreDetails: &finding.ErrorDetails{
			ID:          "undefined_behavior",
			Name:        "Undefined Behavior",
			Description: "An operation has been detected which is undefined by the C/C++ standard. The result will \nbe compiler dependent and is often unpredictable.",
			Severity: &finding.Severity{
				Score: 2.0,
				Level: "Low",
			},
			Mitigation: "Avoid all operations that cause undefined behavior as per the C/C++ standard.",
			Links: []finding.Link{
				{
					Description: "Undefined Behavior Sanitizer",
					URL:         "https://clang.llvm.org/docs/UndefinedBehaviorSanitizer.html#available-checks",
				},
			},
		},
		FuzzTest: "my_fuzz_test",
	}

	err := f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command lists the finding
	stdOut, _, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json", "--server", server.AddressOnHost())
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString(f)
	require.NoError(t, err)
	require.Equal(t, jsonString, stdOut)

	// Check that the command prints extra information
	cmd := newWithOptions(opts)
	_, stdErr, err := cmdutils.ExecuteCommand(t, cmd, os.Stdin, f.Name, "--server", server.AddressOnHost())
	require.NoError(t, err)
	assert.Contains(t, stdErr, "cifuzz found more extensive information about this finding:")
}

func TestPrintRemoteFinding_Authenticated(t *testing.T) {
	t.Setenv("CIFUZZ_API_TOKEN", "token")
	server := mockserver.New(t)
	server.Handlers["/v1/projects"] = mockserver.ReturnResponse(t, mockserver.ProjectsJSON)
	server.Handlers["/v2/error-details"] = mockserver.ReturnResponse(t, mockserver.ErrorDetailsJSON)
	server.Handlers["/v1/projects/my-project/findings"] = mockserver.ReturnResponse(t, mockserver.RemoteFindingsJSON)
	server.Start(t)

	projectDir := testutil.BootstrapEmptyProject(t, "test-print-finding-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Create the expected finding
	f := &finding.Finding{
		Origin:  "CI Sense",
		Name:    "pensive_flamingo",
		Type:    "RUNTIME_ERROR",
		Details: "test_details",
		MoreDetails: &finding.ErrorDetails{
			ID:          "undefined behavior",
			Name:        "Undefined Behavior",
			Description: "An operation has been detected which is undefined by the C/C++ standard. The result will be compiler dependent and is often unpredictable.",
			Severity: &finding.Severity{
				Score: 2.0,
				Level: "Low",
			},
			Mitigation: "Avoid all operations that cause undefined behavior as per the C/C++ standard.",
			Links: []finding.Link{
				{
					Description: "Undefined Behavior Sanitizer",
					URL:         "https://clang.llvm.org/docs/UndefinedBehaviorSanitizer.html#available-checks",
				},
			},
		},
		StackTrace: []*stacktrace.StackFrame{
			{
				Function:   "exploreMe",
				SourceFile: "src/explore_me.cpp",
				Line:       13,
				Column:     11,
			},
		},
		FuzzTest: "my_fuzz_test",
	}

	err := f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command lists the finding
	stdOut, _, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json", "--server", server.AddressOnHost(), "--project", "my-project")
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString(f)
	require.NoError(t, err)
	require.Equal(t, jsonString, stdOut)

	// Check that the command prints extra information
	cmd := newWithOptions(opts)
	_, stdErr, err := cmdutils.ExecuteCommand(t, cmd, os.Stdin, f.Name, "--server", server.AddressOnHost())
	require.NoError(t, err)
	assert.Contains(t, stdErr, "cifuzz found more extensive information about this finding:")
}
