package printflags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/build/other"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
)

func New() []*cobra.Command {
	buildFlags := []string{
		"CFLAGS",
		"CXXFLAGS",
		"LDFLAGS",
		other.EnvFuzzTestCFlags,
		other.EnvFuzzTestCXXFlags,
		other.EnvFuzzTestLDFlags,
	}

	var commands []*cobra.Command
	for _, flags := range buildFlags {
		commands = append(commands, addCommand(flags))
	}

	return commands
}

func addCommand(flags string) *cobra.Command {
	var coverage *bool

	cmd := &cobra.Command{
		Use:    fmt.Sprintf("print-%s", strings.ReplaceAll(strings.ToLower(flags), "_", "-")),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			var env []string
			if *coverage {
				env, err = other.SetCoverageEnv(env, runfiles.Finder)
				if err != nil {
					return err
				}
			} else {
				env, err = other.SetLibFuzzerEnv(env, runfiles.Finder)
				if err != nil {
					return err
				}
			}
			fmt.Print(envutil.Getenv(env, flags))
			return nil
		},
	}
	coverage = cmd.Flags().Bool("coverage", false, "print flags for coverage")

	return cmd
}
