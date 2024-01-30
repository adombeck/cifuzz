package libfuzzer

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/minijail"
	"code-intelligence.com/cifuzz/pkg/options"
	libfuzzer_parser "code-intelligence.com/cifuzz/pkg/parser/libfuzzer"
	"code-intelligence.com/cifuzz/pkg/report"
	fuzzer_runner "code-intelligence.com/cifuzz/pkg/runner"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

const (
	MaxBufferedReports = 10
	sendTimeout        = time.Second * 10
	// ExitGracePeriod is time we give libfuzzer to exit after
	// max_total_time was exceeded.
	// Must be more than 2 seconds, because in the CI it happened that
	// libfuzzer did not exit within 2 seconds.
	ExitGracePeriod = time.Second * 5
)

type RunnerOptions struct {
	Dictionary         string
	EngineArgs         []string
	EnvVars            []string
	FuzzTarget         string
	GeneratedCorpusDir string
	KeepColor          bool
	LibraryDirs        []string
	LogOutput          io.Writer
	ProjectDir         string
	SourceMap          *sourcemap.SourceMap
	ReadOnlyBindings   []string
	ReportHandler      report.Handler
	SeedCorpusDirs     []string
	Timeout            time.Duration
	UseMinijail        bool
	Verbose            bool
	// The path to the coverage binary to use to produce a coverage
	// report after the fuzzer has finished. If empty, no coverage
	// report is produced.
	CoverageBinary      string
	CoverageLibraryDirs []string
	CoverageOutputPath  string
}

func (options *RunnerOptions) ValidateOptions() error {
	if options.UseMinijail {
		if runtime.GOOS != "linux" {
			return errors.Errorf("Minijail is only supported on Linux")
		}

		// To be able to make the fuzz target accessible to minijail,
		// its path must be absolute and all symlinks must be resolved.
		var err error
		options.FuzzTarget, err = filepath.EvalSymlinks(options.FuzzTarget)
		if err != nil {
			return errors.WithStack(err)
		}
		options.FuzzTarget, err = filepath.Abs(options.FuzzTarget)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if options.LogOutput == nil {
		options.LogOutput = os.Stderr
	}

	return nil
}

type Runner struct {
	*RunnerOptions
	SupportJazzer   bool
	SupportJazzerJS bool

	started chan struct{}
	cmd     *executil.Cmd
}

func NewRunner(options *RunnerOptions) *Runner {
	return &Runner{
		RunnerOptions:   options,
		SupportJazzer:   false,
		SupportJazzerJS: false,
		started:         make(chan struct{}, 1),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	err := r.ValidateOptions()
	if err != nil {
		return err
	}

	args := []string{r.FuzzTarget}

	// Tell libfuzzer to exit after the timeout
	timeoutSeconds := strconv.FormatInt(int64(r.Timeout.Seconds()), 10)
	args = append(args, options.LibFuzzerMaxTotalTimeFlag(timeoutSeconds))

	// Tell libfuzzer which dictionary it should use
	if r.Dictionary != "" {
		args = append(args, options.LibFuzzerDictionaryFlag(r.Dictionary))
	}

	// Add user-specified libfuzzer options
	args = append(args, r.EngineArgs...)

	// Tell libfuzzer which corpus directory it should use
	args = append(args, r.GeneratedCorpusDir)

	// Add any seed corpus directories as further positional arguments
	args = append(args, r.SeedCorpusDirs...)

	// Set the directory in which fuzzing artifacts (e.g. crashes) are
	// stored. This must be an absolute path, because else crash files
	// are created in the current working directory, which the fuzz test
	// could change, causing the parser to not find the crash files.
	outputDir, err := os.MkdirTemp("", "libfuzzer-out-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(outputDir)
	args = append(args, options.LibFuzzerArtifactPrefixFlag(outputDir+"/"))

	// The environment to run libfuzzer in
	env, err := r.FuzzerEnvironment()
	if err != nil {
		return err
	}

	if r.UseMinijail {
		libfuzzerArgs := args

		bindings := []*minijail.Binding{
			// The fuzz target must be accessible
			{Source: r.FuzzTarget},
			// The first corpus directory must be writable, because
			// libfuzzer writes new test inputs to it
			{Source: r.GeneratedCorpusDir, Writable: minijail.ReadWrite},
		}

		for _, dir := range r.ReadOnlyBindings {
			bindings = append(bindings, &minijail.Binding{Source: dir})
		}

		for _, dir := range r.SeedCorpusDirs {
			bindings = append(bindings, &minijail.Binding{Source: dir})
		}

		// Set up Minijail
		mj, err := minijail.NewMinijail(&minijail.Options{
			Args:      libfuzzerArgs,
			Bindings:  bindings,
			OutputDir: outputDir,
		})
		if err != nil {
			return err
		}
		defer mj.Cleanup()

		// Use the command which runs libfuzzer via minijail
		args = mj.Args
	}

	return r.RunLibfuzzerAndReport(ctx, args, env)
}

func (r *Runner) RunLibfuzzerAndReport(ctx context.Context, args []string, env []string) error {
	var err error

	// Ideally, libfuzzer exits on its own after the timeout, because we
	// specified `-max_total_time` above. For the case that it does not,
	// we still set up a timeout handler here which sends a SIGTERM and
	// later a SIGKILL if it still didn't exit.
	// To give libfuzzer some time to exit on its own, we send the
	// SIGTERM a bit later than the timeout specified via `-max_total_time`.
	var cmdCtx context.Context
	var cancelCmdCtx context.CancelFunc
	if r.Timeout > 0 {
		terminateTimeout := r.Timeout + ExitGracePeriod
		cmdCtx, cancelCmdCtx = context.WithTimeout(ctx, terminateTimeout)
	} else {
		// No timeout
		cmdCtx, cancelCmdCtx = context.WithCancel(ctx)
	}
	defer cancelCmdCtx()
	r.cmd = executil.CommandContext(cmdCtx, args[0], args[1:]...)
	r.cmd.Env, err = envutil.Copy(os.Environ(), env)
	if err != nil {
		return err
	}

	var stderrPipe io.ReadCloser
	if r.Verbose {
		// Print the command's stdout and stderr via pterm to avoid that
		// the output messes with the pterm output or gets overwritten
		// by it.
		// Note that this causes the command's stdout to be printed to
		// stderr, which is what we want, because we only want reports
		// printed to stdout.
		ptermWriter := log.NewPTermWriter(r.LogOutput)
		r.cmd.Stdout = ptermWriter

		// Write the command's stderr to both a pipe and the pterm
		// writer which prints it to stderr, so that we can parse the
		// output but still allow the caller to observe the status and
		// progress in realtime. If minijail is used, we also filter
		// the output via minijail.OutputFilter
		var stderrOutput io.Writer
		if r.UseMinijail {
			stderrOutput = minijail.NewOutputFilter(ptermWriter)
		} else {
			stderrOutput = ptermWriter
		}
		stderrPipe, err = r.cmd.StderrTeePipe(stderrOutput)
		if err != nil {
			return err
		}
	} else {
		// We use a tee pipe here instead of cmd.StderrPipe because tee
		// pipes allow to call cmd.Wait() before all reads from the pipe
		// have completed. We don't want to write anywhere else but the
		// pipe, so we connect the other end of the tee pipe to io.Discard.
		stderrPipe, err = r.cmd.StderrTeePipe(io.Discard)
		if err != nil {
			return err
		}
	}

	log.Debugf("Command: %s", envutil.QuotedCommandWithEnv(r.cmd.Args, env))
	err = r.cmd.Start()
	if err != nil {
		return err
	}
	r.started <- struct{}{}

	var startupOutput bytes.Buffer
	var startupOutputWriter io.Writer
	if r.UseMinijail {
		startupOutputWriter = minijail.NewOutputFilter(&startupOutput)
	} else {
		startupOutputWriter = &startupOutput
	}
	reporter := libfuzzer_parser.NewLibfuzzerOutputParser(&libfuzzer_parser.Options{
		SupportJazzer:       r.SupportJazzer,
		SupportJazzerJS:     r.SupportJazzerJS,
		KeepColor:           r.KeepColor,
		StartupOutputWriter: startupOutputWriter,
		ProjectDir:          r.ProjectDir,
		SourceMap:           r.SourceMap,
	})
	reportsCh := make(chan *report.Report, MaxBufferedReports)

	// Start a go routine which waits for the command to exit and
	// continuously parses the output
	routines, routinesCtx := errgroup.WithContext(ctx)
	routines.Go(func() error {
		waitErrCh := make(chan error)

		// Wait for the command to exit in a go routine, so that below
		// we can cancel waiting when the context is done
		go func() {
			waitErrCh <- r.cmd.Wait()
		}()

		// Wait until the reporter has finished parsing stderr, so that
		// we can check below whether the reporter has found something
		err := reporter.Parse(routinesCtx, stderrPipe, reportsCh)
		if err != nil {
			return err
		}

		// Tee pipes need to be closed when all reads have completed
		closeErr := stderrPipe.Close()
		if closeErr != nil {
			return errors.WithStack(closeErr)
		}

		select {
		case err := <-waitErrCh:
			if r.cmd.TerminatedAfterContextDone() {
				// The command was terminated because the timeout exceeded. We
				// don't return an error in that case.
				return nil
			}

			// If err is not an ExitError, something unexpected happened
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return err
			}

			if !IsExpectedExitError(err) {
				// Print the stderr output of the fuzzer up to the point where
				// it has been successfully initialized to provide users with
				// the context of this abnormal exit even without verbose mode.
				if !r.Verbose {
					log.Print(startupOutput.String())
				}
				return cmdutils.WrapExecError(errors.WithStack(err), r.cmd.Cmd)
			}

			if !reporter.FindingReported {
				return errors.WithMessagef(err, "libFuzzer exited with expected exit code %d but no finding was reported", exitErr.ExitCode())
			}

			// libFuzzer found an error and exited with an expected error
			// code. We don't want to return an error in that case.
			return nil
		case <-routinesCtx.Done():
			return routinesCtx.Err()
		}
	})

	// Continuously send reports from the reports channel to the
	// receiver. By doing this in a separate go routine and using a
	// buffered reports channel, this allows the LibfuzzerOutputParser
	// function above to not block when creating a report (as long as
	// the buffer is not full).
	routines.Go(func() error {
		senderErrCh := make(chan error, 1)

		go func() {
			senderErrCh <- sendReports(r.ReportHandler, reportsCh)
		}()

		select {
		case err := <-senderErrCh:
			return err
		case <-routinesCtx.Done():
			// The routines context got cancelled, so either the
			// command failed or the reporter encountered an error.
			// We give the sender a few seconds to send pending reports.
			select {
			case err := <-senderErrCh:
				return err
			case <-time.After(sendTimeout):
				return errors.Errorf("Sending reports timed out (%s)", sendTimeout)
			}
		}
	})

	// Routines.Wait() returns an error created by us so it already has a
	// stack trace and we don't want to add another one here
	// nolint: wrapcheck
	return routines.Wait()
}

func (r *Runner) FuzzerEnvironment() ([]string, error) {
	env, err := fuzzer_runner.FuzzerEnvironment()
	if err != nil {
		return nil, err
	}

	env, err = fuzzer_runner.SetLDLibraryPath(env, r.LibraryDirs)
	if err != nil {
		return nil, err
	}

	// Add the user-specified environment variables. We do this after
	// setting our defaults but before setting sanitizer options,
	// because there we take care of overriding options which we need
	// to override and keeping other options.
	if os.Getenv("ASAN_OPTIONS") != "" {
		env, err = envutil.Setenv(env, "ASAN_OPTIONS", os.Getenv("ASAN_OPTIONS"))
		if err != nil {
			return nil, err
		}
	}
	if os.Getenv("UBSAN_OPTIONS") != "" {
		env, err = envutil.Setenv(env, "UBSAN_OPTIONS", os.Getenv("UBSAN_OPTIONS"))
		if err != nil {
			return nil, err
		}
	}
	env, err = fuzzer_runner.AddEnvFlags(env, r.EnvVars)
	if err != nil {
		return nil, err
	}

	env, err = fuzzer_runner.SetCommonUBSANOptions(env)
	if err != nil {
		return nil, err
	}

	env, err = fuzzer_runner.SetCommonASANOptions(env)
	if err != nil {
		return nil, err
	}

	overrideOptions := map[string]string{
		// Per default this is set to false, except for darwin.
		// To have consistent behavior on all supported operating systems
		// we are setting this explicitly to false
		"abort_on_error": "0",
	}
	env, err = fuzzer_runner.SetASANOptions(env, nil, overrideOptions)
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (r *Runner) Cleanup(ctx context.Context) {
	// Wait until the command has been started, else we can't terminate it
	select {
	case <-ctx.Done():
		return
	case <-r.started:
		err := r.cmd.TerminateProcessGroup()
		if err != nil {
			log.Error(err)
		}
	}
}

func sendReports(handler report.Handler, reportsCh <-chan *report.Report) error {
	for r := range reportsCh {
		err := handler.Handle(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func IsExpectedExitError(err error) bool {
	expectedExitCodes := []int{
		fuzzer_runner.SanitizerErrorExitCode,
		fuzzer_runner.LibFuzzerErrorExitCode,
		fuzzer_runner.LibFuzzerOOMExitCode,
		fuzzer_runner.LibFuzzerTimeoutExitCode,
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return sliceutil.Contains(expectedExitCodes, exitErr.ExitCode())
}
