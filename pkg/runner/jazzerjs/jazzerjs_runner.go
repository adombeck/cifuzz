package jazzerjs

import (
	"context"
	"errors"

	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	fuzzer_runner "code-intelligence.com/cifuzz/pkg/runner"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/envutil"
)

type RunnerOptions struct {
	LibfuzzerOptions *libfuzzer.RunnerOptions
	TestPathPattern  string
	TestNamePattern  string
	PackageManager   string
}

func (options *RunnerOptions) ValidateOptions() error {
	err := options.LibfuzzerOptions.ValidateOptions()
	if err != nil {
		return err
	}

	if options.TestPathPattern == "" {
		return errors.New("Test name pattern must be specified.")
	}

	return nil
}

// TODO: define in a central place
type Runner struct {
	*RunnerOptions
	*libfuzzer.Runner
}

func NewRunner(options *RunnerOptions) *Runner {
	libfuzzerRunner := libfuzzer.NewRunner(options.LibfuzzerOptions)
	// TODO: handle different fuzzers properly
	libfuzzerRunner.SupportJazzer = false
	libfuzzerRunner.SupportJazzerJS = true
	return &Runner{options, libfuzzerRunner}
}

func (r *Runner) Run(ctx context.Context) error {
	err := r.ValidateOptions()
	if err != nil {
		return err
	}

	err = r.printDebugVersionInfos()
	if err != nil {
		return err
	}

	args := []string{"npx", "jest"}

	// ---------------------------
	// --- fuzz target arguments -
	// ---------------------------
	args = append(args, options.JazzerJSTestPathPatternFlag(r.TestPathPattern))
	args = append(args, options.JazzerJSTestNamePatternFlag(r.TestNamePattern))
	args = append(args, options.JestTestFailureExitCodeFlag(fuzzer_runner.LibFuzzerErrorExitCode))

	env, err := r.FuzzerEnvironment()
	if err != nil {
		return err
	}

	return r.RunLibfuzzerAndReport(ctx, args, env)
}

func (r *Runner) FuzzerEnvironment() ([]string, error) {
	var env []string

	env, err := fuzzer_runner.AddEnvFlags(env, r.EnvVars)
	if err != nil {
		return nil, err
	}
	env, err = envutil.Setenv(env, "JAZZER_FUZZ", "1")
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (r *Runner) Cleanup(ctx context.Context) {
	r.Runner.Cleanup(ctx)
}

func (r *Runner) printDebugVersionInfos() error {
	jazzerJSVersion, err := dependencies.JazzerJSVersion()
	if err != nil {
		return err
	}
	jestVersion, err := dependencies.JestVersion()
	if err != nil {
		return err
	}

	log.Debugf("JazzerJS version: %s", jazzerJSVersion)
	log.Debugf("Jest version: %s", jestVersion)

	return nil
}
