package root

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	bundleCmd "code-intelligence.com/cifuzz/internal/cmd/bundle"
	containerCmd "code-intelligence.com/cifuzz/internal/cmd/container"
	coverageCmd "code-intelligence.com/cifuzz/internal/cmd/coverage"
	createCmd "code-intelligence.com/cifuzz/internal/cmd/create"
	executeCmd "code-intelligence.com/cifuzz/internal/cmd/execute"
	findingCmd "code-intelligence.com/cifuzz/internal/cmd/finding"
	initCmd "code-intelligence.com/cifuzz/internal/cmd/init"
	integrateCmd "code-intelligence.com/cifuzz/internal/cmd/integrate"
	loginCmd "code-intelligence.com/cifuzz/internal/cmd/login"
	printflagsCmds "code-intelligence.com/cifuzz/internal/cmd/print-flags"
	reloadCmd "code-intelligence.com/cifuzz/internal/cmd/reload"
	remoteRunCmd "code-intelligence.com/cifuzz/internal/cmd/remoterun"
	runCmd "code-intelligence.com/cifuzz/internal/cmd/run"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/version"
	"code-intelligence.com/cifuzz/pkg/log"
)

func New() (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Use:     "cifuzz",
		Version: version.Version,
		// We are using our custom ErrSilent instead to support a more specific
		// error handling
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			log.Infof("cifuzz version %s", version.Version)
			log.Debugf("Running on %s/%s", runtime.GOOS, runtime.GOARCH)

			cmdutils.InitCurrentInvocation(cmd)

			err := cmdutils.Chdir()
			if err != nil {
				return err
			}

			if cmdutils.NeedsConfig(cmd) {
				_, err = config.FindConfigDir()
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("%v\n%s", err, "Use 'cifuzz init' to set up a project for use with cifuzz.")
				}
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("help", false, "Show help for command")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false,
		"Show verbose output on console, can be helpful for debugging")
	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().StringP("directory", "C", "",
		"Change the directory before performing any operations")
	if err := viper.BindPFlag("directory", rootCmd.PersistentFlags().Lookup("directory")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().Bool("no-notifications", false,
		"Turn off desktop notifications")
	if err := viper.BindPFlag("no-notifications", rootCmd.PersistentFlags().Lookup("no-notifications")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().String("style", "pretty", "Defines style for cifuzz (pretty, color, plain)")
	if err := viper.BindPFlag("style", rootCmd.PersistentFlags().Lookup("style")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().Bool("plain", false, "Run cifuzz in pure text mode without any styles")
	if err := viper.BindPFlag("plain", rootCmd.PersistentFlags().Lookup("plain")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.SetFlagErrorFunc(rootFlagErrorFunc)
	rootCmd.SetVersionTemplate(fmt.Sprintf("cifuzz version %s\nRunning on %s/%s\n", version.Version, runtime.GOOS, runtime.GOARCH))

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(loginCmd.New())
	rootCmd.AddCommand(initCmd.New())
	rootCmd.AddCommand(containerCmd.New())
	rootCmd.AddCommand(createCmd.New())
	rootCmd.AddCommand(runCmd.New())
	rootCmd.AddCommand(remoteRunCmd.New())
	rootCmd.AddCommand(reloadCmd.New())
	rootCmd.AddCommand(bundleCmd.New())
	rootCmd.AddCommand(coverageCmd.New())
	rootCmd.AddCommand(findingCmd.New())
	rootCmd.AddCommand(integrateCmd.New())

	for _, cmd := range printflagsCmds.New() {
		rootCmd.AddCommand(cmd)
	}

	if runtime.GOOS != "windows" {
		// The execute command is not supported on Windows
		rootCmd.AddCommand(executeCmd.New())
	}

	return rootCmd, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd, err := New()
	if err != nil {
		fmt.Printf("error while creating root command: %+v", err)
		os.Exit(1)
	}

	var cmd *cobra.Command
	if cmd, err = rootCmd.ExecuteC(); err != nil {
		// Error types that need special handling
		var usageErr *cmdutils.IncorrectUsageError
		var couldBeSandboxError *cmdutils.CouldBeSandboxError
		var signalErr *cmdutils.SignalError
		var silentErr *cmdutils.SilentError

		if errors.As(err, &usageErr) ||
			strings.HasPrefix(err.Error(), "unknown command") ||
			strings.HasPrefix(err.Error(), "invalid argument") ||
			regexp.MustCompile(`(accepts|requires).*arg\(s\)`).MatchString(err.Error()) {
			// Make cmd.Help() print to stderr
			cmd.SetOut(cmd.ErrOrStderr())

			// Print the usage message of the command. We use cmd.Help()
			// here instead of cmd.UsageString() because the latter
			// doesn't include the long description.
			_ = cmd.Help()

			// Add "Invalid Usage" in front of error message
			err = errors.WithMessage(err, "Invalid usage")
		}

		if errors.As(err, &signalErr) {
			// Print error message without stack trace
			log.ErrorMsg(err.Error())

			// Exit with the correct exit status
			os.Exit(128 + int(signalErr.Signal))
		}

		if !errors.As(err, &silentErr) {
			// For any other errors that are not silent (= not expected)
			// we want to print the error and their stack trace in
			// verbose mode (except IncorrectUsageError which should only
			// print the message)
			if errors.As(err, &usageErr) {
				log.ErrorMsg(err.Error())
			} else {
				log.Error(err)
			}
		}

		if errors.As(err, &couldBeSandboxError) {
			// If the error could be caused by sandboxing/minijail we want to
			// print a note after the error message
			msg := `
Note: This error could have occurred due to sandboxing.
If you don't expect this fuzz test to do any harm to the system
accidentally (like overwriting files), you might want to try
running it without sandboxing:

    %s --use-sandbox=false

For more information on cifuzz sandboxing, see:

    https://github.com/CodeIntelligenceTesting/cifuzz/blob/main/docs/Getting-Started.md#sandboxing
`
			log.Notef(msg, shellescape.QuoteCommand(os.Args))
		}

		os.Exit(1)
	}
}

func rootFlagErrorFunc(cmd *cobra.Command, err error) error {
	if errors.Is(err, pflag.ErrHelp) {
		return err
	}
	return cmdutils.WrapIncorrectUsageError(err)
}
