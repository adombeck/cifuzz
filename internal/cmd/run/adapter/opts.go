package adapter

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
)

type RunOptions struct {
	BuildSystem           string        `mapstructure:"build-system"`
	BuildCommand          string        `mapstructure:"build-command"`
	CleanCommand          string        `mapstructure:"clean-command"`
	NumBuildJobs          uint          `mapstructure:"build-jobs"`
	Dictionary            string        `mapstructure:"dict"`
	EngineArgs            []string      `mapstructure:"engine-args"`
	SeedCorpusDirs        []string      `mapstructure:"seed-corpus-dirs"`
	Timeout               time.Duration `mapstructure:"timeout"`
	Interactive           bool          `mapstructure:"interactive"`
	Server                string        `mapstructure:"server"`
	Project               string        `mapstructure:"project"`
	UseSandbox            bool          `mapstructure:"use-sandbox"`
	PrintJSON             bool          `mapstructure:"print-json"`
	BuildOnly             bool          `mapstructure:"build-only"`
	ResolveSourceFilePath bool

	ProjectDir      string
	FuzzTest        string
	TargetMethod    string
	TestNamePattern string
	ArgsToPass      []string

	BuildStdout io.Writer
	BuildStderr io.Writer

	Stdout io.Writer
	Stderr io.Writer
}

func (opts *RunOptions) Validate() error {
	var err error

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

	if opts.BuildSystem == "" {
		opts.BuildSystem, err = config.DetermineBuildSystem(opts.ProjectDir)
		if err != nil {
			return err
		}
	}

	err = config.ValidateBuildSystem(opts.BuildSystem)
	if err != nil {
		return err
	}

	// To build with other build systems, a build command must be provided
	if opts.BuildSystem == config.BuildSystemOther && opts.BuildCommand == "" {
		msg := "Flag \"build-command\" must be set when using build system type \"other\""
		return cmdutils.WrapIncorrectUsageError(errors.New(msg))
	}

	if opts.Timeout != 0 && opts.Timeout < time.Second {
		msg := fmt.Sprintf("invalid argument %q for \"--timeout\" flag: timeout can't be less than a second", opts.Timeout)
		return cmdutils.WrapIncorrectUsageError(errors.New(msg))
	}

	return nil
}
