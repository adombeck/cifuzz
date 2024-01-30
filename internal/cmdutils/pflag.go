package cmdutils

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var BundleFlags = []string{
	"branch",
	"build-command",
	"build-jobs",
	"commit",
	"dict",
	"docker-image",
	"engine-arg",
	"env",
	"seed-corpus",
	"timeout",
}

func ViperMustBindPFlag(key string, flag *pflag.Flag) {
	err := viper.BindPFlag(key, flag)
	if err != nil {
		panic(err)
	}
}

// AddFlags executes the specified Add*Flag functions and returns a
// function which binds all those flags to viper
func AddFlags(cmd *cobra.Command, funcs ...func(cmd *cobra.Command) func()) (bindFlags func()) { // nolint:nonamedreturns
	var bindFlagFuncs []func()
	for _, f := range funcs {
		bindFlagFunc := f(cmd)
		bindFlagFuncs = append(bindFlagFuncs, bindFlagFunc)
	}
	return func() {
		for _, f := range bindFlagFuncs {
			f()
		}
	}
}

func AddAdditionalFilesFlag(cmd *cobra.Command) func() {
	cmd.Flags().StringArray("add", nil,
		"Add a file or directory to the bundle, defined in the format '--add=<source-path>;<target-path>'.\n"+
			"If <target-path> is missing the file gets copied to 'work_dir'.\n"+
			"This flag can be used multiple times.")
	return func() {
		ViperMustBindPFlag("add", cmd.Flags().Lookup("add"))
	}
}

func AddBranchFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("branch", "",
		"Branch name to use in the bundle config.\n"+
			"By default, the currently checked out git branch is used.")
	return func() {
		ViperMustBindPFlag("branch", cmd.Flags().Lookup("branch"))
	}
}

func AddBuildCommandFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("build-command", "",
		"The `command` to build the fuzz test for other build systems.")
	return func() {
		ViperMustBindPFlag("build-command", cmd.Flags().Lookup("build-command"))
	}
}

func AddCleanCommandFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("clean-command", "",
		"The `command` to clean the fuzz test and its dependencies for other build systems.")
	return func() {
		ViperMustBindPFlag("clean-command", cmd.Flags().Lookup("clean-command"))
	}
}

func AddBuildJobsFlag(cmd *cobra.Command) func() {
	cmd.Flags().Uint("build-jobs", 0,
		"Maximum number of concurrent processes to use when building.\n"+
			"If argument is omitted the native build tool's default number is used.")
	cmd.Flags().Lookup("build-jobs").NoOptDefVal = "0"
	return func() {
		ViperMustBindPFlag("build-jobs", cmd.Flags().Lookup("build-jobs"))
	}
}

func AddBuildOnlyFlag(cmd *cobra.Command) func() {
	cmd.Flags().Bool("build-only", false,
		"Only build the fuzz test and don't execute it.")
	return func() {
		ViperMustBindPFlag("build-only", cmd.Flags().Lookup("build-only"))
	}
}

func AddCommitFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("commit", "",
		"Commit to use in the bundle config.\n"+
			"By default, the head of the currently checked out git branch is used.")
	return func() {
		ViperMustBindPFlag("commit", cmd.Flags().Lookup("commit"))
	}
}

func AddDictFlag(cmd *cobra.Command) func() {
	// TODO(afl): Also link to https://github.com/AFLplusplus/AFLplusplus/blob/stable/dictionaries/README.md
	cmd.Flags().String("dict", "",
		"A `file` containing input language keywords or other interesting byte sequences.\n"+
			"This flag is only used if no default dictionary is found for the fuzz test.\n"+
			"See https://llvm.org/docs/LibFuzzer.html#dictionaries.")
	return func() {
		ViperMustBindPFlag("dict", cmd.Flags().Lookup("dict"))
	}
}

func AddDockerImageFlagForContainerCommand(cmd *cobra.Command) func() {
	// Default was originally set to "ubuntu:rolling", but this is not correct
	// It will be set by the bundle command depending on the build system, unless user overrides it
	cmd.Flags().String("docker-image", "",
		`A Docker image which is used as the base for the container image.
The image must contain all the dependencies required to run the fuzz test.
By default, the image is chosen automatically based on the build system
("eclipse-temurin:20" for Java build systems, "ubuntu:rolling" for others).`)
	return func() {
		ViperMustBindPFlag("docker-image", cmd.Flags().Lookup("docker-image"))
	}
}

func AddDockerImageFlagForBundleCommand(cmd *cobra.Command) func() {
	// Default was originally set to "ubuntu:rolling", but this is not correct
	// It will be set by the bundle command depending on the build system, unless user overrides it
	cmd.Flags().String("docker-image", "",
		`Docker image to use in the bundle config. This image will be used when
the bundle is executed on CI Sense.
By default, the image is chosen automatically based on the build system
("eclipse-temurin:20" for Java build systems, "ubuntu:rolling" for others).`)
	return func() {
		ViperMustBindPFlag("docker-image", cmd.Flags().Lookup("docker-image"))
	}
}

func AddEngineArgFlag(cmd *cobra.Command) func() {
	// TODO(afl): Also link to https://www.mankier.com/8/afl-fuzz
	cmd.Flags().StringArray("engine-arg", nil,
		"Command-line `argument` to pass to the fuzzing engine.\n"+
			"See https://llvm.org/docs/LibFuzzer.html#options.\n"+
			"This flag can be used multiple times.\n"+
			"Not supported for Node.js projects.")
	return func() {
		ViperMustBindPFlag("engine-args", cmd.Flags().Lookup("engine-arg"))
	}
}

func AddEnvFlag(cmd *cobra.Command) func() {
	cmd.Flags().StringArray("env", nil,
		"Set environment variable when executing fuzz tests, e.g. '--env `VAR=value`'.\n"+
			"To use the value of VAR in the local environment, use '--env VAR'.\n"+
			"This flag can be used multiple times.")
	return func() {
		ViperMustBindPFlag("env", cmd.Flags().Lookup("env"))
	}
}

func AddInteractiveFlag(cmd *cobra.Command) func() {
	cmd.Flags().Bool("interactive", true, "Toggle interactive prompting in the terminal")
	return func() {
		ViperMustBindPFlag("interactive", cmd.Flags().Lookup("interactive"))
	}
}

func AddPresetFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("preset", "", "Preset for a given environment to execute coverage with necessary flags.\n"+
		"We recommend not using this flag with '--format' or '--output' because the preset will set these accordingly.\n"+
		"If '--format' or '--output' are set, they will overwrite the preset.")
	return func() {
		ViperMustBindPFlag("preset", cmd.Flags().Lookup("preset"))
	}
}

func AddPrintJSONFlag(cmd *cobra.Command) func() {
	cmd.Flags().Bool("json", false, "Print output as JSON")
	return func() {
		ViperMustBindPFlag("print-json", cmd.Flags().Lookup("json"))
	}
}

func AddProjectDirFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("project-dir", "",
		"The project root which is the parent for all the project sources.\n"+
			"Defaults to the directory containing the cifuzz.yaml.")
	return func() {
		ViperMustBindPFlag("project-dir", cmd.Flags().Lookup("project-dir"))
	}
}

func AddResolveSourceFileFlag(cmd *cobra.Command) func() {
	cmd.Flags().BoolP("resolve", "r", false,
		"Argument of the command is a path to a source file instead of a test identifier.\n"+
			"The path can be either absolute or relative to the current working directory and \n"+
			"will be resolved to the identifier of the corresponding fuzz test.")
	return func() {
		ViperMustBindPFlag("resolveSourceFilePath", cmd.Flags().Lookup("resolve"))
	}
}

func AddRegistryFlag(cmd *cobra.Command) func() {
	cmd.Flags().String("registry", "", `The container registry to use for the upload of the container image,
e.g. ghcr.io/my-org/my-project`)
	return func() {
		ViperMustBindPFlag("registry", cmd.Flags().Lookup("registry"))
	}
}

func AddProjectFlag(cmd *cobra.Command) func() {
	// TODO: Make the project name more accessible in the web app (currently
	//       it's only shown in the URL)
	cmd.Flags().StringP("project", "p", "", `The name of the CI Fuzz project you want to start a fuzzing run for,
e.g. "my-project-c170bc17".`)
	return func() {
		ViperMustBindPFlag("project", cmd.Flags().Lookup("project"))
	}
}

func AddSeedCorpusFlag(cmd *cobra.Command) func() {
	// TODO(afl): Also link to https://aflplus.plus/docs/fuzzing_in_depth/#a-collecting-inputs
	cmd.Flags().StringArrayP("seed-corpus", "s", nil,
		"A `directory` containing sample inputs for the code under test,\n"+
			"which is used in addition to inputs found in the inputs\n"+
			"directory of the fuzz test.\n"+
			"See https://github.com/CodeIntelligenceTesting/cifuzz/blob/main/docs/Glossary.md#seed-corpus.\n"+
			"This flag can be used multiple times.")
	return func() {
		ViperMustBindPFlag("seed-corpus-dirs", cmd.Flags().Lookup("seed-corpus"))
	}
}

func AddServerFlag(cmd *cobra.Command) func() {
	cmd.PersistentFlags().String("server", "https://app.code-intelligence.com", "Address of CI Sense")
	return func() {
		ViperMustBindPFlag("server", cmd.Flags().Lookup("server"))
	}
}

func AddTimeoutFlag(cmd *cobra.Command) func() {
	cmd.Flags().Duration("timeout", 0,
		"Maximum time to run the fuzz test, e.g. \"30m\", \"1h\". The default is to run indefinitely.")
	return func() {
		ViperMustBindPFlag("timeout", cmd.Flags().Lookup("timeout"))
	}
}

func AddUseSandboxFlag(cmd *cobra.Command) func() {
	cmd.Flags().Bool("use-sandbox", false,
		"By default, fuzz tests are executed in a sandbox to prevent accidental damage to the system.\n"+
			"Use --use-sandbox=false to run the fuzz test unsandboxed.\n"+
			"Only supported for c/c++ projects on Linux.")
	viper.SetDefault("use-sandbox", runtime.GOOS == "linux")
	return func() {
		ViperMustBindPFlag("use-sandbox", cmd.Flags().Lookup("use-sandbox"))
	}
}
