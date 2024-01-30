package reload

import (
	"runtime"

	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/build/cmake"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
)

type options struct {
	BuildSystem string `mapstructure:"build-system"`
	ProjectDir  string `mapstructure:"project-dir"`
	ConfigDir   string `mapstructure:"config-dir"`
}

// TODO: The reload command allows to reload the fuzz test names used
// for autocompletion from the cmake config. It's only meant as a
// temporary solution until we find a better solution.
type reloadCmd struct {
	*cobra.Command

	opts *options
}

func New() *cobra.Command {
	return newWithOptions(&options{})
}

func newWithOptions(opts *options) *cobra.Command {
	var bindFlags func()
	cmd := &cobra.Command{
		Use:   "reload [flags]",
		Short: "Reload fuzz test metadata",
		// TODO: Write long description
		Long: "",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				return err
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd := reloadCmd{Command: c, opts: opts}
			return cmd.run()
		},
	}

	// Note: If a flag should be configurable via viper as well (i.e.
	//       via cifuzz.yaml and CIFUZZ_* environment variables), bind
	//       it to viper in the PreRun function.
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddProjectDirFlag,
	)

	return cmd
}

func (c *reloadCmd) run() error {
	err := c.checkDependencies()
	if err != nil {
		return err
	}

	if c.opts.BuildSystem == config.BuildSystemCMake {
		return c.reloadCMake()
	} else {
		// Nothing to reload for build systems other than CMake
		return nil
	}
}

func (c *reloadCmd) reloadCMake() error {
	// TODO: Make these configurable
	sanitizers := []string{"address", "undefined"}

	builder, err := cmake.NewBuilder(&cmake.BuilderOptions{
		ProjectDir: c.opts.ProjectDir,
		Sanitizers: sanitizers,
		Stdout:     c.OutOrStdout(),
		Stderr:     c.ErrOrStderr(),
	})
	if err != nil {
		return err
	}

	err = builder.Configure()
	if err != nil {
		return err
	}
	return nil
}

func (c *reloadCmd) checkDependencies() error {
	deps := []dependencies.Key{}
	if c.opts.BuildSystem == config.BuildSystemCMake {
		deps = []dependencies.Key{dependencies.CMake}
		switch runtime.GOOS {
		case "linux", "darwin":
			deps = append(deps, dependencies.Clang)
		case "windows":
			deps = append(deps, dependencies.VisualStudio)
		}
	}
	err := dependencies.Check(deps, c.opts.ProjectDir)
	if err != nil {
		return err
	}
	return nil
}
