package messaging

import "github.com/pterm/pterm"

const UsageWarningText = `You are not authenticated with CI Sense. Please run 'cifuzz login' to authenticate.
Use of this software in production is not allowed without authentication with
CI Sense for any commercial use case. You may continue to use CI Fuzz if you are
using the software for a non-commercial purpose, a demo or a proof of concept.`

func UsageWarning() string {
	return pterm.DefaultBox.WithBoxStyle(
		pterm.NewStyle(pterm.FgRed),
	).Sprint(UsageWarningText)
}
