package root

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
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
				log.Error(err)
				return cmdutils.ErrSilent
			}

			if cmdutils.NeedsConfig(cmd) {
				_, err = config.FindConfigDir()
				if errors.Is(err, os.ErrNotExist) {
					// The project directory doesn't exist, this is an expected
					// error, so we print it and return a silent error to avoid
					// printing a stack trace
					log.Error(err, fmt.Sprintf("%v\nUse 'cifuzz init' to set up a project for use with cifuzz.", err))
					return cmdutils.ErrSilent
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
	rootCmd.AddCommand(createCmd.New())
	rootCmd.AddCommand(runCmd.New())
	rootCmd.AddCommand(remoteRunCmd.New())
	rootCmd.AddCommand(reloadCmd.New())
	rootCmd.AddCommand(bundleCmd.New())
	rootCmd.AddCommand(coverageCmd.New())
	rootCmd.AddCommand(findingCmd.New())
	rootCmd.AddCommand(integrateCmd.New())

	// Only add containers command if envvar CIFUZZ_PRERELEASE is set
	if os.Getenv("CIFUZZ_PRERELEASE") != "" {
		rootCmd.AddCommand(executeCmd.New())
		rootCmd.AddCommand(containerCmd.New())
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
		var silentErr *cmdutils.SilentError

		// Errors that are not ErrSilent are not expected
		// and we want to show the full stacktrace in verbose mode
		if !errors.As(err, &silentErr) {
			icon := "❌ "
			style := pterm.Style{pterm.Bold, pterm.FgRed}
			if log.PlainStyle() {
				icon = ""
				style = pterm.Style{}
			}

			if viper.GetBool("verbose") {
				type stackTracer interface {
					StackTrace() errors.StackTrace
				}
				var st stackTracer
				// Print all error messages (in case of wrapping)
				// but only print the stacktrace of the root error cause
				if errors.As(errors.Cause(err), &st) {
					_, _ = fmt.Fprint(cmd.ErrOrStderr(), style.Sprintf("\n%s%v%+v\n", icon, err, st.StackTrace()))
				} else {
					// Catch cases where we either did not add any stacktrace/wrapped the error
					// or the error does not implement the interface for the stacktracer e.g. os.ErrExist
					_, _ = fmt.Fprint(cmd.ErrOrStderr(), style.Sprintf("\n%s%v\n", icon, err))
				}
			} else {
				supportMsg := "More information can be acquired running the command in verbose mode (-v).\n"
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), style.Sprintf("%s%s\n%s", icon, err, supportMsg))
			}
		}

		// We only want to print the usage message if an ErrIncorrectUsage
		// was returned or it's an error produced by cobra which was
		// caused by incorrect usage
		var usageErr *cmdutils.IncorrectUsageError
		if errors.As(err, &usageErr) ||
			strings.HasPrefix(err.Error(), "unknown command") ||
			regexp.MustCompile(`(accepts|requires).*arg\(s\)`).MatchString(err.Error()) {

			// Ensure that there is an extra newline between the error
			// and the usage message
			if !strings.HasSuffix(err.Error(), "\n") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			}

			// Make cmd.Help() print to stderr
			cmd.SetOut(cmd.ErrOrStderr())
			// Print the usage message of the command. We use cmd.Help()
			// here instead of cmd.UsageString() because the latter
			// doesn't include the long description.
			_ = cmd.Help()
		}

		var couldBeSandboxError *cmdutils.CouldBeSandboxError
		if errors.As(err, &couldBeSandboxError) {
			// Ensure that there is an extra newline between the error
			// and the following message
			if !strings.HasSuffix(err.Error(), "\n") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			}
			msg := `Note: If you don't expect this fuzz test to do any harm to the system
accidentally (like overwriting files), you might want to try
running it without sandboxing:

    %s --use-sandbox=false

For more information on cifuzz sandboxing, see:

    https://github.com/adombeck/cifuzz/blob/main/docs/Getting-Started.md#sandboxing

`
			log.Notef(msg, shellescape.QuoteCommand(os.Args))
		}

		var signalErr *cmdutils.SignalError
		if errors.As(err, &signalErr) {
			os.Exit(128 + int(signalErr.Signal))
		}

		os.Exit(1)
	}
}

func rootFlagErrorFunc(cmd *cobra.Command, err error) error {
	if err == pflag.ErrHelp {
		return err
	}
	return cmdutils.WrapIncorrectUsageError(err)
}
