package init

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const (
	GradleMultiProjectWarningMsg = "For multi-project builds, you should setup cifuzz in the subprojects containing the fuzz tests."
)

type options struct {
	Dir         string
	BuildSystem string
	Interactive bool   `mapstructure:"interactive"`
	Server      string `mapstructure:"server"`
	Project     string `mapstructure:"project"`
	testLang    string
}

func New() *cobra.Command {
	var bindFlags func()
	opts := &options{}
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("init [%s]", strings.Join(supportedInitTestTypes, "|")),
		Short: "Set up a project for use with cifuzz",
		Long: `This command sets up a project for use with cifuzz, creating a
'cifuzz.yaml' config file.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			opts.Interactive = viper.GetBool("interactive")

			var err error
			if opts.Dir == "" {
				opts.Dir, err = os.Getwd()
				if err != nil {
					return errors.WithStack(err)
				}
			}

			if len(args) == 1 {
				opts.testLang = args[0]
			}

			// Override detected build system if test language is specified.
			if opts.testLang != "" {
				// cobra checks for us that opts.testLang is in supportedInitTestTypes
				// because we set ValidArgs below.
				opts.BuildSystem = supportedInitTestTypesMap[opts.testLang]
			} else {
				// Detect and validate buildSystem only when testLang is not specified by the user.
				opts.BuildSystem, err = config.DetermineBuildSystem(opts.Dir)
				if err != nil {
					return err
				}

				err = config.ValidateBuildSystem(opts.BuildSystem)
				if err != nil {
					return err
				}
			}

			if opts.Interactive {
				opts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			}

			if !opts.Interactive && opts.BuildSystem == config.BuildSystemNodeJS && opts.testLang == "" {
				err := errors.New("cifuzz init requires a test language for Node.js projects [js|ts]")
				return cmdutils.WrapIncorrectUsageError(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Project = viper.GetString("project")
			opts.Server = viper.GetString("server")

			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}
			return run(opts)
		},
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: supportedInitTestTypes,
	}

	cmdutils.DisableConfigCheck(cmd)

	// Note: If a flag should be configurable via viper as well (i.e.
	//       via cifuzz.yaml and CIFUZZ_* environment variables), bind
	//       it to viper in the PreRun function.
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddServerFlag,
	)

	return cmd
}

func run(opts *options) error {
	setUpAndMentionBuildSystemIntegrations(opts.Dir, opts.BuildSystem, opts.testLang)
	log.Debugf("Creating config file in directory: %s", opts.Dir)

	configpath, err := config.CreateProjectConfig(opts.Dir, opts.Server, opts.Project)
	if err != nil {
		// explicitly inform the user about an existing config file
		if errors.Is(err, os.ErrExist) && configpath != "" {
			log.Warnf("Config already exists in %s", configpath)
			err = cmdutils.ErrSilent
		}
		log.Error(err, "Failed to create config: %v", err)
		return err
	}

	log.Successf("Configuration saved in %s", fileutil.PrettifyPath(configpath))

	log.Print(`
Use 'cifuzz create' to create your first fuzz test.`)
	return nil
}

func setUpAndMentionBuildSystemIntegrations(dir string, buildSystem string, testLang string) {
	switch buildSystem {
	case config.BuildSystemBazel:
		log.Print(fmt.Sprintf(messaging.Instructions(buildSystem), dependencies.RulesFuzzingWORKSPACEContent, dependencies.CIFuzzBazelCommit))
	case config.BuildSystemCMake:
		// Note: We set NO_SYSTEM_ENVIRONMENT_PATH to avoid that the
		// system-wide cmake package takes precedence over a package
		// from a per-user installation (which is what we want, per-user
		// installations should usually take precedence over system-wide
		// installations).
		//
		// The find_package search procedure is described in
		// https://cmake.org/cmake/help/latest/command/find_package.html#config-mode-search-procedure.
		//
		// Without NO_SYSTEM_ENVIRONMENT_PATH, find_package looks in
		// paths with prefixes from the PATH environment variable in
		// step 5 (omitting any trailing "/bin").
		// The PATH usually includes "/usr/local/bin", which means that
		// find_package searches in "/usr/local/share/cifuzz" in this
		// step, which is the path we use for a system-wide installation.
		//
		// The per-user directory is searched in step 6.
		//
		// With NO_SYSTEM_ENVIRONMENT_PATH, the system-wide installation
		// directory is only searched in step 7.
		log.Print(messaging.Instructions(buildSystem))
	case config.BuildSystemNodeJS:
		if testLang == "" {
			lang, err := getNodeProjectLang()
			if err != nil {
				log.Error(err)
				return
			}
			testLang = lang
		}
		if testLang == "ts" {
			log.Print(messaging.Instructions("nodets"))
		} else {
			log.Print(messaging.Instructions(buildSystem))
		}
	case config.BuildSystemMaven:
		log.Print(messaging.Instructions(buildSystem))
	case config.BuildSystemGradle:
		gradleBuildLanguage, err := config.DetermineGradleBuildLanguage(dir)
		if err != nil {
			log.Error(err)
			return
		}

		isGradleMultiProject, err := config.IsGradleMultiProject(dir)
		if err != nil {
			log.Error(err)
			return
		}
		if isGradleMultiProject {
			log.Warn(GradleMultiProjectWarningMsg)
		}

		log.Print(messaging.Instructions(string(gradleBuildLanguage)))
	}
}

func getNodeProjectLang() (string, error) {
	langOptions := map[string]string{
		"JavaScript": string(config.JavaScript),
		"TypeScript": string(config.TypeScript),
	}

	userSelectedLang, err := dialog.Select("Initialize cifuzz for JavaScript or TypeScript?", langOptions, true)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return userSelectedLang, nil
}

// map of supported test types/build systems for init command. Used to validate input and show args in --help
var supportedInitTestTypesMap = map[string]string{
	"cmake":  config.BuildSystemCMake,
	"maven":  config.BuildSystemMaven,
	"gradle": config.BuildSystemGradle,
	"js":     config.BuildSystemNodeJS,
	"ts":     config.BuildSystemNodeJS,
}

var supportedInitTestTypes = []string{
	"cmake",
	"maven",
	"gradle",
	"js",
	"ts",
}
