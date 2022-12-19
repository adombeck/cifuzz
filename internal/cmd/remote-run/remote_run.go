package remote_run

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type remoteRunOpts struct {
	bundler.Opts `mapstructure:",squash"`
	PrintJSON    bool       `mapstructure:"print-json"`
	ProjectName  string     `mapstructure:"project"`
	LoginOpts    login.Opts `mapstructure:",squash"`

	// Fields which are not configurable via viper (i.e. via cifuzz.yaml
	// and CIFUZZ_* environment variables), by setting
	// mapstructure:"-"
	BundlePath string `mapstructure:"-"`
}

func (opts *remoteRunOpts) Validate() error {
	if !sliceutil.Contains([]string{config.BuildSystemBazel, config.BuildSystemCMake, config.BuildSystemOther}, opts.BuildSystem) {
		err := errors.Errorf(`Starting a remote run is currently not supported for %[1]s projects. If you
are interested in using this feature with %[1]s, please file an issue at
https://github.com/CodeIntelligenceTesting/cifuzz/issues`, cases.Title(language.Und).String(opts.BuildSystem))
		log.Print(err.Error())
		return cmdutils.WrapSilentError(err)
	}

	if opts.BundlePath == "" {
		// We need to build a bundle, so we validate the bundler options
		// as well
		err := opts.Opts.Validate()
		if err != nil {
			return err
		}
	}

	if opts.LoginOpts.Interactive {
		opts.LoginOpts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	}

	return nil
}

type runRemoteCmd struct {
	opts *remoteRunOpts
}

func New() *cobra.Command {
	return newWithOptions(&remoteRunOpts{})
}

func newWithOptions(opts *remoteRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "remote-run [flags] [<fuzz test>]...",
		Short: "Build fuzz tests and run them on a remote fuzzing server",
		Long: `This command builds fuzz tests, packages all runtime artifacts into a
bundle and uploads that to a remote fuzzing server to start a remote
fuzzing run.

If the --bundle flag is used, building and bundling is skipped and the
specified bundle is uploaded to start a remote fuzzing run instead.

This command needs a token to access the API of the remote fuzzing
server. You can specify this token via the CIFUZZ_API_TOKEN environment
variable. If no token is specified, you will be prompted to enter the
token. That token is then stored in ~/.config/cifuzz/access_tokens.json
and used the next time the remote-run command is used.
`,
		ValidArgsFunction: completion.ValidFuzzTests,
		Args:              cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			cmdutils.ViperMustBindPFlag("bundle", cmd.Flags().Lookup("bundle"))

			// Fail early if the platform is not supported
			if runtime.GOOS != "linux" {
				system := cases.Title(language.Und).String(runtime.GOOS)
				if runtime.GOOS == "darwin" {
					system = "macOS"
				}
				err := errors.Errorf(`Starting a remote run is currently only supported on Linux. If you are
interested in using this feature on %s, please file an issue at
https://github.com/CodeIntelligenceTesting/cifuzz/issues`, system)
				log.Print(err.Error())
				return cmdutils.WrapSilentError(err)
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}
			opts.FuzzTests = args

			if opts.ProjectName != "" && !strings.HasPrefix(opts.ProjectName, "projects/") {
				opts.ProjectName = "projects/" + opts.ProjectName
			}

			// If --json was specified, print all build output to stderr
			if opts.PrintJSON {
				opts.Stdout = cmd.ErrOrStderr()
			} else {
				opts.Stdout = cmd.OutOrStdout()
			}
			opts.Stderr = cmd.ErrOrStderr()

			// Check if the server option is a valid URL
			err = api.ValidateURL(opts.LoginOpts.Server)
			if err != nil {
				// See if prefixing https:// makes it a valid URL
				err = api.ValidateURL("https://" + opts.LoginOpts.Server)
				if err != nil {
					log.Error(err, fmt.Sprintf("server %q is not a valid URL", opts.LoginOpts.Server))
				}
				opts.LoginOpts.Server = "https://" + opts.LoginOpts.Server
			}

			// Print warning that flags which only effect the build of
			// the bundle are ignored when an existing bundle is specified
			if opts.BundlePath != "" {
				for _, flag := range cmdutils.BundleFlags {
					if cmd.Flags().Lookup(flag).Changed {
						log.Warnf("Flag --%s is ignored when --bundle is used", flag)
					}
				}
			}

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd := runRemoteCmd{opts: opts}
			return cmd.run()
		},
	}

	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddBranchFlag,
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddCommitFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
	)
	cmd.Flags().StringVar(&opts.BundlePath, "bundle", "", "Path of an existing bundle to start a remote run with.")

	return cmd
}

func (c *runRemoteCmd) run() error {
	var err error

	apiClient := api.APIClient{
		Server: c.opts.LoginOpts.Server,
	}

	token, err := login.Login(c.opts.LoginOpts)
	if err != nil {
		return err
	}

	if c.opts.ProjectName == "" {
		projects, err := apiClient.ListProjects(token)
		if err != nil {
			log.Error(err)
			err = errors.New("Flag \"project\" must be set")
			return cmdutils.WrapIncorrectUsageError(err)
		}

		if c.opts.LoginOpts.Interactive {
			c.opts.ProjectName, err = c.selectProject(projects)
			if err != nil {
				return err
			}
		} else {
			var projectNames []string
			for _, p := range projects {
				projectNames = append(projectNames, strings.TrimPrefix(p.Name, "projects/"))
			}
			if len(projectNames) == 0 {
				log.Warnf("No projects found. Please create a project first at %s.", c.opts.LoginOpts.Server)
				err = errors.New("Flag \"project\" must be set")
				return cmdutils.WrapIncorrectUsageError(err)
			}
			err = errors.New("Flag \"project\" must be set. Valid projects:\n  " + strings.Join(projectNames, "\n  "))
			return cmdutils.WrapIncorrectUsageError(err)
		}
	}

	if c.opts.BundlePath == "" {
		tempDir, err := os.MkdirTemp("", "cifuzz-bundle-")
		if err != nil {
			return errors.WithStack(err)
		}
		defer fileutil.Cleanup(tempDir)
		bundlePath := filepath.Join(tempDir, "fuzz_tests.tar.gz")
		c.opts.BundlePath = bundlePath
		c.opts.OutputPath = bundlePath
		b := bundler.New(&c.opts.Opts)
		err = b.Bundle()
		if err != nil {
			return err
		}
	}

	artifact, err := apiClient.UploadBundle(c.opts.BundlePath, c.opts.ProjectName, token)
	if err != nil {
		var apiErr *api.APIError
		if !errors.As(err, &apiErr) {
			// API calls might fail due to network issues, invalid server
			// responses or similar. We don't want to print a stack trace
			// in those cases.
			log.Error(err)
			return cmdutils.WrapSilentError(err)
		}
		return err
	}

	campaignRunName, err := apiClient.StartRemoteFuzzingRun(artifact, token)
	if err != nil {
		// API calls might fail due to network issues, invalid server
		// responses or similar. We don't want to print a stack trace
		// in those cases.
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	if c.opts.PrintJSON {
		result := struct{ CampaignRun string }{campaignRunName}
		s, err := stringutil.ToJsonString(result)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(os.Stdout, s)
	} else {
		// TODO: Would be nice to be able to link to a page which immediately
		//       shows details about the run, but currently details are only
		//       shown on the "<fuzz target>/edit" page, which lists all runs
		//       of the fuzz target.
		log.Successf(`Successfully started fuzzing run. To view findings and coverage, open:

    %s/dashboard/%s/overview

`, c.opts.LoginOpts.Server, campaignRunName)
	}

	return nil
}

func (c *runRemoteCmd) selectProject(projects []*api.Project) (string, error) {
	// Let the user select a project
	var displayNames []string
	var names []string
	for _, p := range projects {
		displayNames = append(displayNames, p.DisplayName)
		names = append(names, p.Name)
	}
	maxLen := stringutil.MaxLen(displayNames)
	items := map[string]string{}
	for i := range displayNames {
		key := fmt.Sprintf("%-*s [%s]", maxLen, displayNames[i], strings.TrimPrefix(names[i], "projects/"))
		items[key] = names[i]
	}

	if len(items) == 0 {
		err := errors.Errorf("No projects found. Please create a project first at %s.", c.opts.LoginOpts.Server)
		log.Error(err)
		return "", cmdutils.WrapSilentError(err)
	}

	projectName, err := dialog.Select("Select the project you want to start a fuzzing run for", items)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return projectName, nil
}
