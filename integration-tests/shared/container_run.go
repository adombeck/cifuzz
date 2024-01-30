package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mattn/go-zglob"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/cicheck"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestContainerRun(t *testing.T, cifuzzRunner *CIFuzzRunner, imageTag string, options *RunOptions) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test because the container run command is not supported on Windows")
	}

	// TODO: Get Docker to work on macOS GitHub Runners
	if runtime.GOOS == "darwin" && cicheck.IsCIEnvironment() {
		t.Skip("Skipping test because Docker is not supported on macOS GitHub Runners")
	}

	// Remove inputs from the inputs directory (if it was created by a
	// previous test) to be able to test that new seeds are created in
	// the generated corpus (they won't be created if the fuzzer exits
	// early because it finds a crash while initializing with the
	// existing seeds).
	// We use zglob to find all inputs directories because the directory
	// name depends on the language and fuzz test name.
	cInputDirs, err := zglob.Glob(filepath.Join(cifuzzRunner.DefaultWorkDir, "**", "*inputs"))
	require.NoError(t, err)
	javaInputDirs, err := zglob.Glob(filepath.Join(cifuzzRunner.DefaultWorkDir, "**", "*Inputs"))
	require.NoError(t, err)
	for _, inputsDir := range append(cInputDirs, javaInputDirs...) {
		exists, err := fileutil.Exists(inputsDir)
		require.NoError(t, err)
		if exists {
			log.Printf("Removing inputs from %s", inputsDir)
			// Remove all files in the inputs directory
			err = os.RemoveAll(inputsDir)
			require.NoError(t, err)
			// Create an empty inputs directory, because some of the
			// tests (e.g. the build system other test) require the
			// inputs directory to exist.
			err = os.Mkdir(inputsDir, 0755)
			require.NoError(t, err)
		}
	}

	// Build the cifuzz base image which is used by the container run
	// command to ensure that the base image contains the latest version
	// of the cifuzz binary.
	buildCIFuzzBaseImage(t)

	// Create a temporary directory which we mount into the container to
	// be able to access the generated corpus files and the coverage
	// report.
	outputDir := testutil.MkdirTemp(t, "", "cifuzz-container-run-output-*")

	options.Command = []string{"container", "run"}
	options.Args = append(options.Args,
		"--docker-image", imageTag,
		// Mount the output directory into the container
		"--bind", fmt.Sprintf("%s:/output", outputDir),
		// All other arguments are passed to the fuzz container. This
		// requires two "--" because arguments after the first "--" are
		// used as build system arguments and arguments after the second
		// "--" are used as container arguments.
		"--", "--",
		// Specify the generated corpus dir
		"--generated-corpus-dir", "/output/generated-corpus",
		// Produce an LCOV coverage report
		"--coverage-output-path", "/output/coverage.lcov",
	)

	cifuzzRunner.Run(t, options)

	// Check that files were created in the corpus directory
	entries, err := os.ReadDir(filepath.Join(outputDir, "generated-corpus"))
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Check that the corpus directory only contains files and no directories
	for _, entry := range entries {
		require.False(t, entry.IsDir())
	}

	// Check that the LCOV coverage report was created
	_, err = os.Stat(filepath.Join(outputDir, "coverage.lcov"))
	require.NoError(t, err)
}

func BuildDockerImage(t *testing.T, tag string, buildDir string) {
	var err error
	cmd := exec.Command("docker", "build", "-t", tag, buildDir)
	cmd.Env, err = envutil.Setenv(os.Environ(), "DOCKER_BUILDKIT", "1")
	require.NoError(t, err)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}

// This is not a test but a helper function so gocritic should not
// complain that it's not of the form TestXXX(t *testing.T).
//
//nolint:gocritic
func buildCIFuzzBaseImage(t *testing.T) {
	var err error

	dir, err := os.Getwd()
	require.NoError(t, err)

	// Find the integration-tests directory
	for {
		if filepath.Base(dir) == "integration-tests" {
			break
		}
		dir = filepath.Dir(dir)
	}
	// Go one level up to get the cifuzz directory
	dir = filepath.Dir(dir)

	cmd := exec.Command("make", "build-container-image")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}
