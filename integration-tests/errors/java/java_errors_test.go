package java

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_JavaErrors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testdataTmp := shared.CopyTestdataDir(t, "java_cpp")
	t.Cleanup(func() { fileutil.Cleanup(testdataTmp) })

	// This testdata is in a separate project due to external dependencies that
	// need to be downloaded for compilation and slow down every testcase.
	testdataLDAPAndSQLTmp := shared.CopyCustomTestdataDir(t, "testdata-sql-ldap", "java_cpp_sql_ldap")

	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))
	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath: cifuzz,
	}

	testCases := []struct {
		id       string
		fuzzTest string
		args     []string
		workdir  string
	}{
		{
			id:       "java_out_of_bounds",
			fuzzTest: "com.collection.ArrayOutOfBoundsFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "load_arbitrary_library",
			fuzzTest: "com.collection.LoadArbitraryLibraryFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "negative_array_size",
			fuzzTest: "com.collection.NegativeArraySizeFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "null_pointer",
			fuzzTest: "com.collection.NullPointerExceptionFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "number_format",
			fuzzTest: "com.collection.NumberFormatExceptionFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "os_command_injection",
			fuzzTest: "com.collection.OSCommandInjectionFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "regex_injection",
			fuzzTest: "com.collection.RegexInjectionFuzzTest::fuzzTestInsecureQuote",
			workdir:  testdataTmp,
		},
		{
			id:       "regex_injection",
			fuzzTest: "com.collection.RegexInjectionFuzzTest::fuzzTestICanonEQ",
			workdir:  testdataTmp,
		},
		{
			id:       "remote_code_execution",
			fuzzTest: "com.collection.RemoteCodeExecutionFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "xpath_injection",
			fuzzTest: "com.collection.XPathInjectionFuzzTest",
			workdir:  testdataTmp,
		},
		{
			id:       "timeout",
			fuzzTest: "com.collection.TimeoutFuzzTest",
			args:     []string{"--engine-arg=-timeout=1s", "--engine-arg=-runs=-1"},
			workdir:  testdataTmp,
		},
		{
			id:       "ldap_injection",
			fuzzTest: "com.collection.LDAPInjectionFuzzTest::dnFuzzTest",
			workdir:  testdataLDAPAndSQLTmp,
		},
		{
			id:       "ldap_injection",
			fuzzTest: "com.collection.LDAPInjectionFuzzTest::searchFuzzTest",
			workdir:  testdataLDAPAndSQLTmp,
		},
		{
			id:       "sql_injection",
			fuzzTest: "com.collection.SQLInjectionFuzzTest",
			workdir:  testdataLDAPAndSQLTmp,
		},
		{
			id:       "java_exception",
			fuzzTest: "com.collection.ExceptionFuzzTest::fuzzTestException",
			workdir:  testdataTmp,
		},
		{
			id:       "java_exception",
			fuzzTest: "com.collection.ExceptionFuzzTest::fuzzTestSecurityException",
			workdir:  testdataTmp,
		},
		{
			id:       "server_side_request_forgery",
			fuzzTest: "com.collection.ServerSideRequestForgeryFuzzTest",
			workdir:  testdataTmp,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			cifuzzRunner.Run(t, &shared.RunOptions{
				FuzzTest: tc.fuzzTest,
				Args:     tc.args,
				WorkDir:  tc.workdir,
			})

			// Call findings command, get json output and check for finding id
			_, findingsJSON := cifuzzRunner.CommandOutput(t, "findings", &shared.CommandOptions{
				Args: []string{
					"--json",
					"--interactive=false",
				},
				WorkDir: tc.workdir,
			})

			var findings []finding.Finding
			err := json.Unmarshal([]byte(findingsJSON), &findings)
			require.NoError(t, err)
			idFound := false
			for _, f := range findings {
				if f.MoreDetails.ID == tc.id {
					idFound = true
					break
				}
			}
			assert.True(t, idFound, "id '%s' not found", tc.id)
		})
	}
}
