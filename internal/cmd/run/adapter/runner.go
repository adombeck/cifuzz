package adapter

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/java"
	"code-intelligence.com/cifuzz/internal/build/java/gradle"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/pkg/java/sourcemap"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type FuzzerRunner interface {
	Run(context.Context) error
	Cleanup(context.Context)
}

func ExecuteFuzzerRunner(runner FuzzerRunner) error {
	// Handle cleanup (terminating the fuzzer process) when receiving
	// termination signals
	signalHandlerCtx, cancelSignalHandler := context.WithCancel(context.Background())
	routines, routinesCtx := errgroup.WithContext(signalHandlerCtx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	var signalErr error
	routines.Go(func() error {
		select {
		case <-routinesCtx.Done():
			return nil
		case s := <-sigs:
			log.Warnf("Received %s", s.String())
			signalErr = cmdutils.NewSignalError(s.(syscall.Signal))
			runner.Cleanup(routinesCtx)
			return signalErr
		}
	})

	// Run the fuzzer
	routines.Go(func() error {
		defer cancelSignalHandler()
		return runner.Run(routinesCtx)
	})

	err := routines.Wait()
	// We use a separate variable to pass signal errors, because when
	// a signal was received, the first goroutine terminates the second
	// one, resulting in a race of which returns an error first. In that
	// case, we always want to print the signal error, not the
	// "Unexpected exit code" error from the runner.
	if signalErr != nil {
		return signalErr
	}

	var execErr *cmdutils.ExecError
	if errors.As(err, &execErr) {
		// If the error is expected because libFuzzer might fail due to user
		// configuration, we return the execErr directly
		return execErr
	}

	// Routines.Wait() returns an error created by us so it already has a
	// stack trace and we don't want to add another one here
	// nolint: wrapcheck
	return err
}

func runLibfuzzer(opts *RunOptions, buildResult *build.BuildResult, reportHandler *reporthandler.ReportHandler) error {
	var err error

	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	log.Infof("Running %s", style.Sprintf(opts.FuzzTest))
	log.Debugf("Executable: %s", buildResult.Executable)

	var libraryPaths []string
	if runtime.GOOS != "windows" {
		var err error
		libraryPaths, err = ldd.LibraryPaths(buildResult.Executable)
		if err != nil {
			return err
		}
	}

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists).
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		opts.SeedCorpusDirs = append(opts.SeedCorpusDirs, buildResult.SeedCorpus)
	}

	// If user-specified dictionary is not set, use
	// implicit dictionary from buildResult (if it exists).
	if opts.Dictionary == "" {
		exists, err := fileutil.Exists(buildResult.Dictionary)
		if err != nil {
			return err
		}
		if exists {
			opts.Dictionary = buildResult.Dictionary
		}
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		Dictionary:         opts.Dictionary,
		EngineArgs:         opts.EngineArgs,
		EnvVars:            []string{"NO_CIFUZZ=1"},
		FuzzTarget:         buildResult.Executable,
		LibraryDirs:        libraryPaths,
		GeneratedCorpusDir: buildResult.GeneratedCorpus,
		KeepColor:          !opts.PrintJSON && !log.PlainStyle(),
		ProjectDir:         opts.ProjectDir,
		ReadOnlyBindings:   []string{buildResult.BuildDir},
		ReportHandler:      reportHandler,
		SeedCorpusDirs:     opts.SeedCorpusDirs,
		Timeout:            opts.Timeout,
		UseMinijail:        opts.UseSandbox,
		Verbose:            viper.GetBool("verbose"),
	}

	// TODO: Only set ReadOnlyBindings if buildResult.BuildDir != ""
	return ExecuteFuzzerRunner(libfuzzer.NewRunner(runnerOpts))
}

func runJazzer(opts *RunOptions, buildResult *build.BuildResult, reportHandler *reporthandler.ReportHandler) error {
	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	log.Infof("Running %s", style.Sprintf(opts.FuzzTest+"::"+opts.TargetMethod))

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists).
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		opts.SeedCorpusDirs = append(opts.SeedCorpusDirs, buildResult.SeedCorpus)
	}

	// Create source map
	sourceDirs, err := java.SourceDirs(opts.ProjectDir, opts.BuildSystem)
	if err != nil {
		return err
	}
	testDirs, err := java.TestDirs(opts.ProjectDir, opts.BuildSystem)
	if err != nil {
		return err
	}

	// In case of multi-module projects the project root directory is
	// determined by the build system.
	rootDir := opts.ProjectDir
	if opts.BuildSystem == config.BuildSystemGradle {
		rootDir, err = gradle.GetRootDirectory(opts.ProjectDir)
		if err != nil {
			return err
		}
	}
	sourceMap, err := sourcemap.CreateSourceMap(rootDir, append(sourceDirs, testDirs...))
	if err != nil {
		return err
	}

	var fuzzerRunner FuzzerRunner

	runnerOpts := &jazzer.RunnerOptions{
		TargetClass:  opts.FuzzTest,
		TargetMethod: opts.TargetMethod,
		ClassPaths:   buildResult.RuntimeDeps,
		LibfuzzerOptions: &libfuzzer.RunnerOptions{
			Dictionary:         opts.Dictionary,
			EngineArgs:         opts.EngineArgs,
			EnvVars:            []string{"NO_CIFUZZ=1"},
			FuzzTarget:         buildResult.Executable,
			GeneratedCorpusDir: buildResult.GeneratedCorpus,
			KeepColor:          !opts.PrintJSON && !log.PlainStyle(),
			ProjectDir:         opts.ProjectDir,
			SourceMap:          sourceMap,
			ReadOnlyBindings:   []string{buildResult.BuildDir},
			ReportHandler:      reportHandler,
			SeedCorpusDirs:     opts.SeedCorpusDirs,
			Timeout:            opts.Timeout,
			UseMinijail:        opts.UseSandbox,
			Verbose:            viper.GetBool("verbose"),
		},
	}

	fuzzerRunner = jazzer.NewRunner(runnerOpts)
	return ExecuteFuzzerRunner(fuzzerRunner)
}
