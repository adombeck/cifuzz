package run

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmd/bundle"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type containerRunOpts struct {
	bundler.Opts  `mapstructure:",squash"`
	PrintJSON     bool     `mapstructure:"print-json"`
	Interactive   bool     `mapstructure:"interactive"`
	Server        string   `mapstructure:"server"`
	ContainerPath string   `mapstructure:"container"`
	BindMounts    []string `mapstructure:"bind-mounts"`
	BuildOnly     bool     `mapstructure:"build-only"`
}

type containerRunCmd struct {
	*cobra.Command
	opts *containerRunOpts
}

func New() *cobra.Command {
	return newWithOptions(&containerRunOpts{})
}

func (opts *containerRunOpts) Validate() error {
	return opts.Opts.Validate()
}

func newWithOptions(opts *containerRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "run [flags] <fuzz test> [--] [<build system arg>...] [--] [<container arg>...] ",
		Short: "Build and run a Fuzz Test container image locally",
		Long: `This command builds and runs a Fuzz Test container image locally.
It can be used as a containerized version of the 'cifuzz bundle' command, where the
container is built and run locally instead of being pushed to a CI Sense server.`,
		ValidArgsFunction: completion.ValidFuzzTests,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			// Check correct number of fuzz test args (exactly one)
			var lenFuzzTestArgs int
			var buildSystemArgs []string
			if cmd.ArgsLenAtDash() != -1 {
				lenFuzzTestArgs = cmd.ArgsLenAtDash()
				buildSystemArgs = args[cmd.ArgsLenAtDash():]
				args = args[:cmd.ArgsLenAtDash()]
			} else {
				lenFuzzTestArgs = len(args)
			}
			if lenFuzzTestArgs != 1 {
				msg := fmt.Sprintf("Exactly one <fuzz test> argument must be provided, got %d", lenFuzzTestArgs)
				return cmdutils.WrapIncorrectUsageError(errors.New(msg))
			}

			var containerArgs []string
			// If the args contain another '--', treat all args after it as
			// container args.
			if index := stringutil.Index(buildSystemArgs, "--"); index != -1 {
				containerArgs = buildSystemArgs[index+1:]
				buildSystemArgs = buildSystemArgs[:index]
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}

			fuzzTests, err := resolve.FuzzTestArguments(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				return err
			}
			opts.FuzzTests = fuzzTests
			opts.BuildSystemArgs = buildSystemArgs
			opts.ContainerArgs = containerArgs

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd := &containerRunCmd{Command: c, opts: opts}
			return cmd.run()
		},
	}
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddAdditionalFilesFlag,
		cmdutils.AddBranchFlag,
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddCleanCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddCommitFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddDockerImageFlagForContainerCommand,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)
	cmd.Flags().StringVar(&opts.ContainerPath, "container", "", "Path of an existing container to start a run with.")
	cmd.Flags().StringArrayVar(&opts.BindMounts, "bind", nil, "Bind mount a directory from the host into the container. "+
		"Format: --bind <src-path>:<dest-path>")
	cmd.Flags().BoolVar(&opts.BuildOnly, "build-only", false, "Only build the container image, don't run it.")

	// For now the --bind flag is only used for tests, so we hide it from the help output.
	err := cmd.Flags().MarkHidden("bind")
	if err != nil {
		panic(err)
	}

	return cmd
}

func (c *containerRunCmd) run() error {
	authenticated, err := auth.HasValidToken(c.opts.Server)
	if err != nil {
		var connErr *api.ConnectionError
		if !errors.As(err, &connErr) {
			return err
		} else {
			log.Debugf("Connection error: %v", connErr)
		}
	}
	if !authenticated {
		log.Infof(messaging.UsageWarning())
	}

	buildOutput := c.OutOrStdout()
	if c.opts.PrintJSON {
		// We only want JSON output on stdout, so we print the build
		// output to stderr.
		buildOutput = c.ErrOrStderr()
	}
	buildPrinter := logging.NewBuildPrinter(buildOutput, log.ContainerBuildInProgressMsg)
	imageID, err := c.buildContainerImage(buildOutput)
	if err != nil {
		buildPrinter.StopOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	buildPrinter.StopOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	if c.opts.BuildOnly {
		return nil
	}

	containerID, err := container.Create(imageID, c.opts.PrintJSON, c.opts.BindMounts, c.opts.ContainerArgs)
	if err != nil {
		return err
	}

	err = container.Run(containerID, c.OutOrStdout(), c.ErrOrStderr())
	if err != nil {
		return err
	}

	return nil
}

func (c *containerRunCmd) buildContainerImage(buildOutput io.Writer) (string, error) {
	err := bundle.SetUpBundleLogging(buildOutput, c.ErrOrStderr(), &c.opts.Opts)
	if err != nil {
		return "", err
	}

	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", errors.WithMessage(err, "Failed to create bundle")
	}

	return container.BuildImageFromBundle(bundlePath)
}
