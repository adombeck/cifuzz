package remoterun

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
)

type containerRemoteRunOpts struct {
	bundler.Opts `mapstructure:",squash"`
	Interactive  bool   `mapstructure:"interactive"`
	Server       string `mapstructure:"server"` // CI Sense
	PrintJSON    bool   `mapstructure:"print-json"`
	Project      string `mapstructure:"project"` // CI Sense
	Registry     string `mapstructure:"registry"`
}

type containerRemoteRunCmd struct {
	*cobra.Command
	opts      *containerRemoteRunOpts
	apiClient *api.APIClient
}

func New() *cobra.Command {
	return newWithOptions(&containerRemoteRunOpts{})
}

func newWithOptions(opts *containerRemoteRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "remote-run",
		Short: "Build and run a Fuzz Test container image on a CI server",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			var argsToPass []string
			if cmd.ArgsLenAtDash() != -1 {
				argsToPass = args[cmd.ArgsLenAtDash():]
				args = args[:cmd.ArgsLenAtDash()]
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}

			// check for required registry flag
			err = cmd.MarkFlagRequired("registry")
			if err != nil {
				return errors.WithStack(err)
			}

			fuzzTests, err := resolve.FuzzTestArguments(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				return err
			}
			opts.FuzzTests = fuzzTests
			opts.BuildSystemArgs = argsToPass

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}

			cmd := &containerRemoteRunCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server)
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
		cmdutils.AddRegistryFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)

	return cmd
}

func (c *containerRemoteRunCmd) run() error {
	var err error

	token, err := auth.EnsureValidToken(c.opts.Server)
	if err != nil {
		return err
	}

	if c.opts.Project == "" {
		projects, err := c.apiClient.ListProjects(token)
		if err != nil {
			log.Error(err)
			err = errors.New("Flag \"project\" must be set")
			return cmdutils.WrapIncorrectUsageError(err)
		}

		if c.opts.Interactive {
			c.opts.Project, err = dialog.ProjectPickerWithOptionNew(projects, "Select the project you want to start a fuzzing run for:", c.apiClient, token)
			if err != nil {
				return err
			}

			if c.opts.Project == "<<cancel>>" {
				log.Info("Container remote run cancelled.")
				return nil
			}

			// this will ask users via a y/N prompt if they want to persist the
			// project choice
			err = dialog.AskToPersistProjectChoice(c.opts.Project)
			if err != nil {
				return err
			}
		} else {
			err = errors.New("Flag 'project' must be set.")
			return cmdutils.WrapIncorrectUsageError(err)
		}
	}

	buildOutput := c.OutOrStdout()
	if c.opts.PrintJSON {
		// We only want JSON output on stdout, so we print the build
		// output to stderr.
		buildOutput = c.ErrOrStderr()
	}
	buildPrinter := logging.NewBuildPrinter(buildOutput, log.ContainerBuildInProgressMsg)
	imageID, err := c.buildImage()
	if err != nil {
		buildPrinter.StopOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	buildPrinter.StopOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	err = container.UploadImage(imageID, c.opts.Registry)
	if err != nil {
		return err
	}

	imageID = c.opts.Registry + ":" + imageID
	response, err := c.apiClient.PostContainerRemoteRun(imageID, c.opts.Project, c.opts.FuzzTests, token)
	if err != nil {
		return err
	}

	responseJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	if c.opts.PrintJSON {
		_, _ = fmt.Fprintln(os.Stdout, string(responseJSON))
	}

	addr, err := cmdutils.BuildURLFromParts(c.opts.Server, "dashboard", "projects", c.opts.Project, "runs")
	if err != nil {
		return err
	}

	log.Successf(`Successfully started fuzzing run. To view findings and coverage, open:
    %s`, addr)

	return nil
}

func (c *containerRemoteRunCmd) buildImage() (string, error) {
	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", err
	}

	return container.BuildImageFromBundle(bundlePath)
}
