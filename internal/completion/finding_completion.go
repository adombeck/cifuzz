package completion

import (
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
)

// ValidFindings can be used as a cobra ValidArgsFunction that completes
// finding names.
func ValidFindings(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Change the directory if the `--directory` flag was set
	err := cmdutils.Chdir()
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	// Find the project directory
	projectDir, err := config.FindConfigDir()
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	findings, err := finding.LocalFindings(projectDir, nil)
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	var findingNames []string
	for _, f := range findings {
		findingNames = append(findingNames, f.Name+"\t"+f.ShortDescription())
	}
	return findingNames, cobra.ShellCompDirectiveNoFileComp
}
