package log

import (
	"io"
	"os"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

const (
	BuildInProgressMsg        string = "Build in progress..."
	BuildInProgressSuccessMsg string = "Build in progress... Done."
	BuildInProgressErrorMsg   string = "Build in progress... Error."

	BundleInProgressMsg        string = "Bundle in progress..."
	BundleInProgressSuccessMsg string = "Bundle in progress... Done."
	BundleInProgressErrorMsg   string = "Bundle in progress... Error."

	ContainerBuildInProgressMsg        string = "Building fuzz container..."
	ContainerBuildInProgressSuccessMsg string = "Building fuzz container... Done."
	ContainerBuildInProgressErrorMsg   string = "Building fuzz container... Error."
)

var activeSpinnerPrinter *SpinnerPrinter

type SpinnerPrinter struct {
	*pterm.SpinnerPrinter
}

func NewSpinnerPrinter(style *pterm.Style, output io.Writer, msg string) *SpinnerPrinter {
	spinner := pterm.DefaultSpinner.WithWriter(output)
	if style != nil {
		spinner.Style = style
		spinner.MessageStyle = style
	}
	p := &SpinnerPrinter{spinner}

	// error can be ignored here because pterm doesn't return one
	p.SpinnerPrinter, _ = spinner.Start(msg)

	activeSpinnerPrinter = p
	return p
}

func (p *SpinnerPrinter) Update(msg string) {
	if p != nil && msg != "" {
		p.SpinnerPrinter.UpdateText(msg)
	}
}

func (p *SpinnerPrinter) StopWithMessage(msg string) {
	if msg != "" {
		p.SpinnerPrinter.UpdateText(msg)
	}
	p.SpinnerPrinter.RemoveWhenDone = false
	p.Stop()
}

func (p *SpinnerPrinter) Stop() {
	activeSpinnerPrinter = nil
	// error can be ignored here since pterm doesn't return one
	_ = p.SpinnerPrinter.Stop()
}

func ShouldUseSpinnerPrinter() bool {
	return !PlainStyle() && term.IsTerminal(int(os.Stdout.Fd()))
}

func UpdateCurrentSpinnerPrinter(msg string) {
	if msg != "" && activeSpinnerPrinter != nil {
		activeSpinnerPrinter.UpdateText(msg)
	}
}
