package execute

import "github.com/spf13/cobra"

// The execute command is not supported on Windows.
func New() *cobra.Command {
	return nil
}
