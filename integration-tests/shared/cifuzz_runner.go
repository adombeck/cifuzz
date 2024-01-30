package shared

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
)

type CIFuzzRunner struct {
	CIFuzzPath      string
	DefaultWorkDir  string
	DefaultFuzzTest string
}

type CommandOptions struct {
	WorkDir string
	Env     []string
	Args    []string
}

// CommandOutput runs "cifuzz <command> <args>" and returns stderr and output.
func (r *CIFuzzRunner) CommandOutput(t *testing.T, command string, opts *CommandOptions) (string, string) {
	t.Helper()

	if opts == nil {
		opts = &CommandOptions{}
	}

	var args []string
	// Empty command means that the root command should be executed
	if command != "" {
		args = append(args, command)
	}
	args = append(args, opts.Args...)

	if opts.WorkDir == "" {
		opts.WorkDir = r.DefaultWorkDir
	}

	cmd := executil.Command(r.CIFuzzPath, args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Command: %s", cmd.String())
	err := cmd.Run()
	require.NoError(t, err)

	return stderr.String(), stdout.String()
}

// Command runs "cifuzz <command> <args>" and returns stderr output as lines.
func (r *CIFuzzRunner) Command(t *testing.T, command string, opts *CommandOptions) []string {
	t.Helper()

	if opts == nil {
		opts = &CommandOptions{}
	}

	var args []string
	// Empty command means that the root command should be executed
	if command != "" {
		args = append(args, command)
	}
	args = append(args, opts.Args...)

	if opts.WorkDir == "" {
		opts.WorkDir = r.DefaultWorkDir
	}

	cmd := executil.Command(r.CIFuzzPath, args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	stderrPipe, err := cmd.StderrTeePipe(os.Stderr)
	require.NoError(t, err)
	defer func() {
		err = stderrPipe.Close()
		require.NoError(t, err)
	}()

	log.Printf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)

	scanner := bufio.NewScanner(stderrPipe)
	var stdErrOutput []string
	for scanner.Scan() {
		stdErrOutput = append(stdErrOutput, scanner.Text())
	}

	return stdErrOutput
}

func FilterForInstructions(lines []string) []string {
	var linesToAdd []string
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") {
			linesToAdd = append(linesToAdd, strings.TrimSpace(line))
		}
	}
	return linesToAdd
}

// CommandWithFilterForInstructions runs "cifuzz <command> <args>" and
// returns any indented lines which the command prints to stderr
// (which we expect to be lines which should be added to some source or config file).
func (r *CIFuzzRunner) CommandWithFilterForInstructions(t *testing.T, command string, opts *CommandOptions) []string {
	allLines := r.Command(t, command, opts)
	return FilterForInstructions(allLines)
}

type RunOptions struct {
	FuzzTest string
	WorkDir  string
	Env      []string
	Command  []string
	Args     []string

	ExpectedOutputs              []*regexp.Regexp
	UnexpectedOutput             *regexp.Regexp
	TerminateAfterExpectedOutput bool
	ExpectError                  bool
}

func (r *CIFuzzRunner) Run(t *testing.T, opts *RunOptions) {
	t.Helper()
	var err error

	if opts.Command == nil {
		opts.Command = []string{"run"}
	}

	env := opts.Env
	env, err = envutil.Setenv(env, "CIFUZZ_INTERACTIVE", "false")
	require.NoError(t, err)
	env, err = envutil.Setenv(env, "CIFUZZ_NO_NOTIFICATIONS", "true")
	require.NoError(t, err)
	_, env = testutil.SetupCoverage(t, env, "integration")

	if opts.WorkDir == "" {
		opts.WorkDir = r.DefaultWorkDir
	}

	if opts.FuzzTest == "" {
		opts.FuzzTest = r.DefaultFuzzTest
	}

	runCtx, closeRunCtx := context.WithCancel(context.Background())
	defer closeRunCtx()
	args := append(append(opts.Command,
		[]string{
			"-v", opts.FuzzTest,
			"--engine-arg=-seed=1",
			"--engine-arg=-runs=1000000",
		}...),
		opts.Args...,
	)

	cmd := executil.CommandContext(
		runCtx,
		r.CIFuzzPath,
		args...,
	)
	cmd.Dir = opts.WorkDir
	cmd.Env, err = envutil.Copy(os.Environ(), env)
	require.NoError(t, err)
	stdoutPipe, err := cmd.StdoutTeePipe(os.Stdout)
	require.NoError(t, err)
	stderrPipe, err := cmd.StderrTeePipe(os.Stderr)
	require.NoError(t, err)

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	TerminateOnSignal(t, cmd)

	log.Printf("Command: %s", envutil.QuotedCommandWithEnv(cmd.Args, env))
	err = cmd.Start()
	require.NoError(t, err)

	waitErrCh := make(chan error)
	// Wait for the command to exit in a go routine, so that below
	// we can cancel waiting when the context is done
	go func() {
		waitErrCh <- cmd.Wait()
	}()

	// Check that the output contains the expected output
	outputChecker := outputChecker{
		mutex:                        &sync.Mutex{},
		lenExpectedOutputs:           len(opts.ExpectedOutputs),
		expectedOutputs:              opts.ExpectedOutputs,
		unexpectedOutput:             opts.UnexpectedOutput,
		terminateAfterExpectedOutput: opts.TerminateAfterExpectedOutput,
		termationFunc: func() {
			err := cmd.TerminateProcessGroup()
			require.NoError(t, err)
		},
	}

	routines := errgroup.Group{}
	routines.Go(func() error {
		// cifuzz progress messages go to stdout.
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			outputChecker.checkOutput(t, scanner.Text())
		}
		err = stdoutPipe.Close()
		require.NoError(t, err)
		return nil
	})

	routines.Go(func() error {
		// Fuzzer output goes to stderr.
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			outputChecker.checkOutput(t, scanner.Text())
		}
		err = stderrPipe.Close()
		require.NoError(t, err)
		return nil
	})

	select {
	case waitErr := <-waitErrCh:

		err = routines.Wait()
		require.NoError(t, err)

		if outputChecker.hasCalledTerminationFunc && executil.IsTerminatedExitErr(waitErr) {
			return
		}
		if opts.ExpectError {
			require.Error(t, waitErr)
		} else {
			require.NoError(t, waitErr)
		}
	case <-runCtx.Done():
		require.NoError(t, runCtx.Err())
	}

	require.True(t, outputChecker.hasSeenExpectedOutputs, "Did not see %q in fuzzer output", outputChecker.expectedOutputs)
}

type outputChecker struct {
	mutex                        *sync.Mutex
	lenExpectedOutputs           int
	expectedOutputs              []*regexp.Regexp
	unexpectedOutput             *regexp.Regexp
	terminateAfterExpectedOutput bool
	termationFunc                func()
	hasSeenExpectedOutputs       bool
	hasCalledTerminationFunc     bool
}

func (c *outputChecker) checkOutput(t *testing.T, line string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.unexpectedOutput != nil {
		if c.unexpectedOutput.MatchString(line) {
			require.FailNowf(t, "Unexpected output", "Seen unexpected output %v in line: %s", c.unexpectedOutput.String(), line)
		}
	}

	var remainingExpectedOutputs []*regexp.Regexp
	for _, expectedOutput := range c.expectedOutputs {
		if !expectedOutput.MatchString(line) {
			remainingExpectedOutputs = append(remainingExpectedOutputs, expectedOutput)
		}
	}
	c.expectedOutputs = remainingExpectedOutputs

	if len(remainingExpectedOutputs) == 0 {
		c.hasSeenExpectedOutputs = true
		if c.terminateAfterExpectedOutput {
			c.hasCalledTerminationFunc = true
			c.termationFunc()
		}
	}

}
