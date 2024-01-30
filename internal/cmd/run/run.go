package run

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmd/run/adapter"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

type runCmd struct {
	*cobra.Command

	opts         *adapter.RunOptions
	apiClient    *api.APIClient
	errorDetails []*finding.ErrorDetails

	reportHandler *reporthandler.ReportHandler
}

func New() *cobra.Command {
	opts := &adapter.RunOptions{}
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "run [flags] <fuzz test> [--] [<build system arg>...] ",
		Short: "Build and run a fuzz test",
		Long: `This command builds and executes a fuzz test. The usage of this command
depends on the build system configured for the project.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("CMake") + `
  <fuzz test> is the name of the fuzz test defined in the add_fuzz_test
  command in your CMakeLists.txt.

  Command completion for the <fuzz test> argument is supported when the
  fuzz test was built before or after running 'cifuzz reload'.

  The --build-command flag is ignored.

  Additional CMake arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- -G Ninja

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

  The default dictionary

    <fuzz test>.dict

  is used automatically if no other dictionary is specified
  by using the --dict flag.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Bazel") + `
  <fuzz test> is the name of the cc_fuzz_test target as defined in your
  BUILD file, either as a relative or absolute Bazel label.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  Additional Bazel arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- --sandbox_debug

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Maven/Gradle") + `
  <fuzz test> is the name of the class containing the fuzz test(s).
  If the fuzz test class contains multiple fuzz tests,
  you can use <fuzz test>::<method name> to specify a single fuzz
  test.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  The inputs found in the directory

    src/test/resources/.../<fuzz test>Inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Node.js") + `
  <fuzz test> is a regex pattern that matches against all paths
  containing fuzz test files.
  If the matched fuzz test file contains multiple fuzz tests,
  you can use <fuzz test>:<test name>
  to specify a regex that matches the fuzz test name.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  The inputs found in the directory

    <fuzz test>.fuzz

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Other build systems") + `
  <fuzz test> is either the path or basename of the fuzz test executable
  created by the build command. If it's the basename, it will be searched
  for recursively in the current working directory.

  A command which builds the fuzz test executable must be provided via
  the --build-command flag or the build-command setting in cifuzz.yaml.

  The value specified for <fuzz test> is made available to the build
  command in the FUZZ_TEST environment variable. For example:

    echo "build-command: make clean && make \$FUZZ_TEST" >> cifuzz.yaml
    cifuzz run my_fuzz_test

  To avoid cleaning the build artifacts after building each fuzz test, you
  can provide a clean command using the --clean-command flag or specifying
  the "clean-command" option in cifuzz.yaml. The clean command is then
  executed once before building the fuzz tests.

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

  The default dictionary

    <fuzz test>.dict

  is used automatically if no other dictionary is specified
  by using the --dict flag.

`,
		ValidArgsFunction: completion.ValidFuzzTests,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			// Check correct number of fuzz test args (exactly one)
			var lenFuzzTestArgs int
			var argsToPass []string
			if cmd.ArgsLenAtDash() != -1 {
				lenFuzzTestArgs = cmd.ArgsLenAtDash()
				argsToPass = args[cmd.ArgsLenAtDash():]
				args = args[:cmd.ArgsLenAtDash()]
			} else {
				lenFuzzTestArgs = len(args)
			}
			if lenFuzzTestArgs != 1 {
				msg := fmt.Sprintf("Exactly one <fuzz test> argument must be provided, got %d", lenFuzzTestArgs)
				return cmdutils.WrapIncorrectUsageError(errors.New(msg))
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}

			if sliceutil.Contains(
				[]string{config.BuildSystemMaven, config.BuildSystemGradle},
				opts.BuildSystem,
			) {
				// Check if the fuzz test is a method of a class
				// And remove method from fuzz test argument
				if strings.Contains(args[0], "::") {
					split := strings.Split(args[0], "::")
					args[0], opts.TargetMethod = split[0], split[1]
				}
			} else if opts.BuildSystem == config.BuildSystemNodeJS {
				// Check if the fuzz test contains a filter for the test name
				if strings.Contains(args[0], ":") {
					split := strings.Split(args[0], ":")
					args[0], opts.TestNamePattern = split[0], strings.ReplaceAll(split[1], "\"", "")
				}
			}

			fuzzTests, err := resolve.FuzzTestArguments(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				return err
			}
			opts.FuzzTest = fuzzTests[0]

			opts.ArgsToPass = argsToPass

			if opts.PrintJSON {
				// We only want JSON output on stdout, so we print the build
				// output to stderr.
				opts.BuildStdout = cmd.ErrOrStderr()
			} else {
				opts.BuildStdout = cmd.OutOrStdout()
			}
			opts.BuildStderr = cmd.OutOrStderr()

			opts.Stdout = cmd.OutOrStdout()
			opts.Stderr = cmd.OutOrStderr()

			if logging.ShouldLogBuildToFile() {
				opts.BuildStdout, err = logging.BuildOutputToFile(opts.ProjectDir, []string{opts.FuzzTest})
				if err != nil {
					return err
				}
				opts.BuildStderr = opts.BuildStdout
			}

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}

			cmd := runCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server)
			return cmd.run()
		},
	}

	// Note: If a flag should be configurable via cifuzz.yaml as well,
	// bind it to viper in the PreRunE function.
	funcs := []func(cmd *cobra.Command) func(){
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddCleanCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddBuildOnlyFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddUseSandboxFlag,
		cmdutils.AddResolveSourceFileFlag,
	}
	bindFlags = cmdutils.AddFlags(cmd, funcs...)
	return cmd
}

func (c *runCmd) run() error {
	errorDetails, token, err := auth.TryGetErrorDetailsAndToken(c.opts.Server)
	if err != nil {
		return err
	}
	c.errorDetails = errorDetails

	adapter, err := adapter.NewAdapter(c.opts)
	if err != nil {
		return err
	}
	defer adapter.Cleanup()

	err = adapter.CheckDependencies(c.opts.ProjectDir)
	if err != nil {
		return err
	}

	c.reportHandler, err = adapter.Run(c.opts)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && c.opts.UseSandbox {
			return cmdutils.WrapCouldBeSandboxError(err)
		}
		return err
	}
	// happens when `--build-only` was called
	if c.reportHandler == nil && err == nil {
		return nil
	}
	c.reportHandler.ErrorDetails = errorDetails

	c.reportHandler.PrintCrashingInputNote()
	err = c.reportHandler.PrintFinalMetrics()
	if err != nil {
		return err
	}

	// We need this check, otherwise we might hang forever in CI
	if c.opts.Project == "" && !c.opts.Interactive {
		log.Info("Skipping upload of findings because no project was specified and running in non-interactive mode.")
		return nil
	}
	if c.opts.Project == "" && !term.IsTerminal(int(os.Stdout.Fd())) {
		log.Info("Skipping upload of findings because no project was specified and stdout is not a terminal.")
		return nil
	}

	// check if there are findings that should be uploaded
	if token != "" && len(c.reportHandler.Findings) > 0 {
		err = c.uploadFindings(c.getFuzzTestNameForCampaignRun(), c.opts.BuildSystem, c.reportHandler.FirstMetrics, c.reportHandler.LastMetrics, token)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *runCmd) uploadFindings(fuzzTarget, buildSystem string, firstMetrics *report.FuzzingMetric, lastMetrics *report.FuzzingMetric, token string) error {
	projects, err := c.apiClient.ListProjects(token)
	if err != nil {
		return err
	}

	project := c.opts.Project
	if project == "" {
		// ask user to select project
		project, err = dialog.ProjectPickerWithOptionNew(projects, "Select the project you want to upload findings to:", c.apiClient, token)
		if err != nil {
			return cmdutils.WrapSilentError(err)
		}

		// if the user cancels the project selection, we don't want to upload
		if project == "<<cancel>>" {
			log.Info("Upload cancelled by user.")
			return nil
		}

		// this will ask users via a y/N prompt if they want to persist the
		// project choice
		err = dialog.AskToPersistProjectChoice(project)
		if err != nil {
			return cmdutils.WrapSilentError(err)
		}
	} else {
		// check if project exists on server
		found := false
		for _, p := range projects {
			if url.PathEscape(p.Name) == project {
				found = true
				break
			}
		}

		if !found {
			message := fmt.Sprintf(`Project %s does not exist on server %s.
Findings have *not* been uploaded. Please check the 'project' entry in your cifuzz.yml.`, project, c.opts.Server)
			return errors.New(message)
		}
	}

	// create campaign run on server for selected project
	campaignRunName, fuzzingRunName, err := c.apiClient.CreateCampaignRun(project, token, fuzzTarget, buildSystem, firstMetrics, lastMetrics)
	if err != nil {
		return err
	}

	// upload findings
	for _, finding := range c.reportHandler.Findings {
		if c.errorDetails != nil {
			finding.EnhanceWithErrorDetails(c.errorDetails)
		}
		err = c.apiClient.UploadFinding(project, fuzzTarget, campaignRunName, fuzzingRunName, finding, token)
		if err != nil {
			return err
		}
		// after a finding has been uploaded, we can delete the local copy
		err = finding.Remove(c.opts.ProjectDir)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("Failed to remove finding %s", finding.Name))
		}
	}
	log.Notef("Uploaded %d findings to CI Sense at: %s", len(c.reportHandler.Findings), c.opts.Server)
	log.Infof("You can view the findings at %s/dashboard/%s/findings?origin=cli", c.opts.Server, campaignRunName)

	return nil
}

func (c *runCmd) getFuzzTestNameForCampaignRun() string {
	if c.opts.BuildSystem == config.BuildSystemMaven ||
		c.opts.BuildSystem == config.BuildSystemGradle {
		return fmt.Sprintf("%s::%s", c.opts.FuzzTest, c.opts.TargetMethod)
	}

	return c.opts.FuzzTest
}
