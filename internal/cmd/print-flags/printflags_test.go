package printflags

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestPrintBuildFlags(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	includeDir, err := filepath.Abs(filepath.Join(installDir, "include"))
	require.NoError(t, err)
	dumperDir, err := filepath.Abs(filepath.Join(installDir, "lib", "dumper.o"))
	require.NoError(t, err)
	fuzzTestLDFlagsNoCov := fmt.Sprintf("-fsanitize=fuzzer %s", dumperDir)
	if runtime.GOOS != "darwin" {
		fuzzTestLDFlagsNoCov = fmt.Sprintf("-Wl,--wrap=__sanitizer_set_death_callback %s", fuzzTestLDFlagsNoCov)
	}

	testCases := []struct {
		command        string
		coverageFlag   bool
		expectedOutput string
	}{
		{
			command:        "print-cflags",
			coverageFlag:   false,
			expectedOutput: "-g -Og -fno-omit-frame-pointer -DFUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION -UNDEBUG -fsanitize=fuzzer-no-link -fsanitize=address,undefined -fsanitize-recover=address -fsanitize-address-use-after-scope -U_FORTIFY_SOURCE",
		},
		{
			command:        "print-cflags",
			coverageFlag:   true,
			expectedOutput: "-g -Og -fno-omit-frame-pointer -DFUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION -UNDEBUG -fprofile-instr-generate -fcoverage-mapping -U_FORTIFY_SOURCE -mllvm -runtime-counter-relocation",
		},
		{
			command:        "print-cxxflags",
			coverageFlag:   false,
			expectedOutput: "-g -Og -fno-omit-frame-pointer -DFUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION -UNDEBUG -fsanitize=fuzzer-no-link -fsanitize=address,undefined -fsanitize-recover=address -fsanitize-address-use-after-scope -U_FORTIFY_SOURCE",
		},
		{
			command:        "print-cxxflags",
			coverageFlag:   true,
			expectedOutput: "-g -Og -fno-omit-frame-pointer -DFUZZING_BUILD_MODE_UNSAFE_FOR_PRODUCTION -UNDEBUG -fprofile-instr-generate -fcoverage-mapping -U_FORTIFY_SOURCE -mllvm -runtime-counter-relocation",
		},
		{
			command:        "print-ldflags",
			coverageFlag:   false,
			expectedOutput: "-fsanitize=address,undefined",
		},
		{
			command:        "print-ldflags",
			coverageFlag:   true,
			expectedOutput: "-fprofile-instr-generate",
		},
		{
			command:        "print-fuzz-test-cflags",
			coverageFlag:   false,
			expectedOutput: fmt.Sprintf("-I%s", includeDir),
		},
		{
			command:        "print-fuzz-test-cflags",
			coverageFlag:   true,
			expectedOutput: fmt.Sprintf("-I%s", includeDir),
		},
		{
			command:        "print-fuzz-test-cxxflags",
			coverageFlag:   false,
			expectedOutput: fmt.Sprintf("-I%s", includeDir),
		},
		{
			command:        "print-fuzz-test-cxxflags",
			coverageFlag:   true,
			expectedOutput: fmt.Sprintf("-I%s", includeDir),
		},
		{
			command:        "print-fuzz-test-ldflags",
			coverageFlag:   false,
			expectedOutput: fuzzTestLDFlagsNoCov,
		},
		{
			command:        "print-fuzz-test-ldflags",
			coverageFlag:   true,
			expectedOutput: "-fsanitize=fuzzer",
		},
	}

	for _, tc := range testCases {
		args := []string{tc.command}
		if tc.coverageFlag {
			args = append(args, "--coverage")
		}

		cmd := exec.Command(cifuzz, args...)
		output, err := cmd.Output()

		require.NoError(t, err)
		assert.Equal(t, tc.expectedOutput, string(output))
	}
}
