package bundler

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

type Opts struct {
	Branch          string        `mapstructure:"branch"`
	BuildCommand    string        `mapstructure:"build-command"`
	CleanCommand    string        `mapstructure:"clean-command"`
	BuildSystem     string        `mapstructure:"build-system"`
	NumBuildJobs    uint          `mapstructure:"build-jobs"`
	Commit          string        `mapstructure:"commit"`
	Dictionary      string        `mapstructure:"dict"`
	DockerImage     string        `mapstructure:"docker-image"`
	EngineArgs      []string      `mapstructure:"engine-args"`
	Env             []string      `mapstructure:"env"`
	SeedCorpusDirs  []string      `mapstructure:"seed-corpus-dirs"`
	Timeout         time.Duration `mapstructure:"timeout"`
	ProjectDir      string        `mapstructure:"project-dir"`
	ConfigDir       string        `mapstructure:"config-dir"`
	AdditionalFiles []string      `mapstructure:"add"`

	// Fields which are not configurable via viper (i.e. via cifuzz.yaml
	// and CIFUZZ_* environment variables), by setting
	// mapstructure:"-"
	FuzzTests       []string  `mapstructure:"-"`
	OutputPath      string    `mapstructure:"-"`
	BuildSystemArgs []string  `mapstructure:"-"`
	ContainerArgs   []string  `mapstructure:"-"`
	Stdout          io.Writer `mapstructure:"-"`
	Stderr          io.Writer `mapstructure:"-"`
	BuildStdout     io.Writer `mapstructure:"-"`
	BuildStderr     io.Writer `mapstructure:"-"`

	tempDir string `mapstructure:"-"`

	ResolveSourceFilePath bool
	BundleBuildLogFile    string
}

func (opts *Opts) Validate() error {
	var err error

	// Ensure that the fuzz tests contain no duplicates
	opts.FuzzTests = sliceutil.RemoveDuplicates(opts.FuzzTests)

	opts.SeedCorpusDirs, err = cmdutils.ValidateSeedCorpusDirs(opts.SeedCorpusDirs)
	if err != nil {
		return err
	}

	if opts.Dictionary != "" {
		// Check if the dictionary exists and can be accessed
		_, err = os.Stat(opts.Dictionary)
		if err != nil {
			return errors.Wrapf(err, "Failed to access dictionary %s", opts.Dictionary)
		}
	}

	if opts.BuildSystem == config.BuildSystemBazel {
		// We don't support building a bundle with bazel without any
		// specified fuzz tests
		if len(opts.FuzzTests) == 0 {
			msg := `At least one <fuzz test> argument must be provided`
			return cmdutils.WrapIncorrectUsageError(errors.New(msg))
		}

		// Evaluate any target patterns which users might have provided
		patterns := opts.FuzzTests
		opts.FuzzTests, err = cmdutils.EvaluateBazelTargetPatterns(patterns)
		if err != nil {
			return err
		}

		if len(opts.FuzzTests) == 0 {
			return errors.Errorf("No valid targets found for patterns: %s", strings.Join(patterns, " "))
		}
	}

	if opts.BuildSystem == config.BuildSystemOther {
		// To build with other build systems, a build command must be provided
		if opts.BuildCommand == "" {
			msg := "Flag \"build-command\" must be set when using build system type \"other\""
			return cmdutils.WrapIncorrectUsageError(errors.New(msg))
		}
		// To build with other build systems, the fuzz tests need to be
		// specified (because there is no way for us to figure out which
		// fuzz tests exist).
		if len(opts.FuzzTests) == 0 {
			msg := `At least one <fuzz test> argument must be provided when using the build
system type "other"`
			return cmdutils.WrapIncorrectUsageError(errors.New(msg))
		}
	}

	if opts.Timeout != 0 && opts.Timeout < time.Second {
		msg := fmt.Sprintf("invalid argument %q for \"--timeout\" flag: timeout can't be less than a second", opts.Timeout)
		return cmdutils.WrapIncorrectUsageError(errors.New(msg))
	}

	// If an env var doesn't contain a "=", it means the user wants to
	// use the value from the current environment
	var env []string
	for _, e := range opts.Env {
		if strings.Contains(e, "=") {
			// The environment variable contains a "=", so we use it
			env = append(env, e)
			continue
		}
		if os.Getenv(e) == "" {
			// The variable does not contain a "=" and is not set in the
			// current environment, so we ignore it
			continue
		}
		// Use the variable with the value from the current environment
		env = append(env, fmt.Sprintf("%s=%s", e, os.Getenv(e)))
	}
	opts.Env = env

	return nil
}
