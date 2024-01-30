package create

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/stubs"
)

type createOpts struct {
	BuildSystem string `mapstructure:"build-system"`
	Interactive bool   `mapstructure:"interactive"`

	outputPath string
	testType   config.FuzzTestType
}

func (opts *createOpts) Validate() error {
	err := config.ValidateBuildSystem(opts.BuildSystem)
	if err != nil {
		return err
	}

	if opts.Interactive {
		opts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	}

	if !opts.Interactive && opts.testType == "" {
		err := errors.New(fmt.Sprintf("Missing argument [%s]", strings.Join(maps.Values(config.SupportedTestTypes), "|")))
		return cmdutils.WrapIncorrectUsageError(err)
	}

	return nil
}

type createCmd struct {
	*cobra.Command

	opts *createOpts
}

func New() *cobra.Command {
	return newWithOptions(&createOpts{})
}

func newWithOptions(opts *createOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("create [%s]", strings.Join(maps.Values(config.SupportedTestTypes), "|")),
		Short: "Create a new fuzz test",
		Long: `This command creates a new templated fuzz test source file in the current directory.
After running this command, you should edit the created file in order to
make it call the functions you want to fuzz. You can then execute the
fuzz test via 'cifuzz run'.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			if len(args) == 1 {
				opts.testType = config.FuzzTestType(args[0])
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd := createCmd{
				Command: c,
				opts:    opts,
			}
			return cmd.run()
		},
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: maps.Values(config.SupportedTestTypes),
	}

	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddInteractiveFlag,
	)
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "File path of new fuzz test")

	return cmd
}

func (c *createCmd) run() error {
	var err error
	// get test type
	if c.opts.testType == "" {
		c.opts.testType, err = c.getTestType()
		if err != nil {
			return err
		}
	}
	log.Debugf("Selected fuzz test type: %s", c.opts.testType)

	if c.opts.outputPath == "" {
		c.opts.outputPath, err = stubs.FuzzTestFilename(c.opts.testType)
		if err != nil {
			return err
		}
	}
	log.Debugf("Output path: %s", c.opts.outputPath)

	c.checkDependencies()

	// create stub
	err = stubs.Create(c.opts.outputPath, c.opts.testType)
	if err != nil {
		return errors.WithMessagef(err, "Failed to create fuzz test stub %s", c.opts.outputPath)
	}

	// show success message
	log.Successf("Created fuzz test stub %s", c.opts.outputPath)
	log.Print(`
Note: Fuzz tests can be put anywhere in your repository, but it makes sense
to keep them close to the tested code - just like regular unit tests.`)

	c.printBuildSystemInstructions()

	return nil
}

// getTestType returns the test type (selected by argument or input dialog)
func (c *createCmd) getTestType() (config.FuzzTestType, error) {
	userSelectedType, err := dialog.Select("Select type of the fuzz test", config.SupportedTestTypes, true)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return config.FuzzTestType(userSelectedType), nil
}

func (c *createCmd) printBuildSystemInstructions() {
	filename := filepath.Base(c.opts.outputPath)
	// Printing build system instructions is best-effort: Do not fail on errors.
	switch c.opts.BuildSystem {
	case config.BuildSystemBazel:
		log.Printf(`
Define a bazel target for the fuzz test by adding the following to the
BUILD.bazel file:

    load("@rules_fuzzing//fuzzing:cc_defs.bzl", "cc_fuzz_test")

    cc_fuzz_test(
        name = "%[1]s",
        srcs = ["%[2]s"],
        corpus = glob(
            ["%[1]s_inputs/**"],
            allow_empty = True,
        ) + select({
            "@cifuzz//:collect_coverage": glob([".%[1]s_cifuzz_corpus/**"], allow_empty = True),
            "//conditions:default": [],
        }),
        deps = ["@cifuzz"],
    )

`, strings.TrimSuffix(filename, filepath.Ext(filename)), filename)
	case config.BuildSystemCMake:
		log.Printf(`
Create a CMake target for the fuzz test as follows - it behaves just like
a regular add_executable(...):

    add_fuzz_test(%s %s)

`, strings.TrimSuffix(filename, filepath.Ext(filename)), filename)

	case config.BuildSystemOther:
		log.Printf(`
It seems like you're not using a build system which cifuzz has special
integration support for, so you'll have to configure your build system
yourself in order to build the fuzz test and specify the command which
produces the fuzz test executable via the '--build-command' flag or the
'build-command' option in the cifuzz.yaml. See 'cifuzz run --help' for
more information about the build command.

The FUZZ_TEST_CFLAGS and FUZZ_TEST_LDFLAGS environment variables are
set by cifuzz when building the fuzz test, please make sure that
$FUZZ_TEST_CFLAGS is passed as a command-line argument to the compiler
and $FUZZ_TEST_LDFLAGS to the linker.`)
	}
}

func (c *createCmd) checkDependencies() {
	var deps []dependencies.Key
	switch c.opts.BuildSystem {
	case config.BuildSystemBazel:
		deps = []dependencies.Key{dependencies.Bazel}
	case config.BuildSystemCMake:
		deps = []dependencies.Key{dependencies.CMake}
		switch runtime.GOOS {
		case "linux", "darwin":
			deps = append(deps, dependencies.Clang)
		case "windows":
			deps = append(deps, dependencies.VisualStudio)
		}
	case config.BuildSystemOther:
		deps = []dependencies.Key{dependencies.Clang}
	}
	err := dependencies.Check(deps, "")
	if err != nil {
		// we ignore errors here because this command has no actual
		// dependencies and we just want to give recommendations
		// instead of letting the command fail
		log.Error(err)
	}
}
