package log

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var disableColor bool

// Output is the primary outlet for the log to write to.
var Output io.Writer

// VerboseSecondaryOutput captures the complete verbose output
// of the primary output, even when the command is not called
// in verbose mode. It provides a secondary output option.
var VerboseSecondaryOutput io.Writer

func init() {
	Output = os.Stderr
	// Disable color if stderr is not a terminal. We don't use
	// the color flag here because that would disable color for all
	// pterm and color methods, but we might want to use color in output
	// printed to stdout (if stdout is a terminal).
	disableColor = !term.IsTerminal(int(os.Stderr.Fd()))
}

func log(style pterm.Style, icon string, a ...any) {
	s := fmt.Sprint(a...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}

	// Can't do this check in init() because the flag is not yet set at that point
	switch {
	case PlainStyle():
		// Disable all colors (this also influences all other pterm writers)
		// Don't use DisableStyling() because otherwise the spinners won't work anymore
		pterm.DisableColor()
	case viper.GetString("style") == "color":
		s = style.Sprint(s)
	default:
		s = icon + s
		s = style.Sprint(s)
	}

	if disableColor {
		s = pterm.RemoveColorFromString(s)
	}

	// Clear the updating printer output if any. We don't use
	// pterm.Fprint here, which also tries to clear spinner printer
	// output, because that only works when the spinner printer and this
	// function write to the same output stream, which is not always the
	// case, because we let the spinner printer write to stdout unless
	// --json was used. The advantage of that is that it allows piping
	// stderr to a log file while still seeing the output that's mostly
	// relevant during execution. But if that continues to add complexity
	// to our code, we might want to reassess the cost/benefit.
	if ActiveUpdatingPrinter != nil {
		ActiveUpdatingPrinter.Clear()
	}

	// If a progress spinner is currently running, we have to stop it,
	// then print the log and start it again to have a clean output
	// If we don't do this, the spinner will remain on the console
	// between the logs
	if activeSpinnerPrinter != nil {
		// We only need to set this if we have to restart the spinner
		activeSpinnerPrinter.RemoveWhenDone = true
		_ = activeSpinnerPrinter.SpinnerPrinter.Stop()

		_, _ = fmt.Fprint(Output, s)
		logToSecondaryOutput(a...)

		activeSpinnerPrinter.SpinnerPrinter, _ = activeSpinnerPrinter.Start(activeSpinnerPrinter.Text)
		return
	}

	_, _ = fmt.Fprint(Output, s)
	logToSecondaryOutput(a...)
}

func logToSecondaryOutput(a ...any) {
	if VerboseSecondaryOutput == nil {
		// Do nothing if this is not set
		return
	}

	s := fmt.Sprint(a...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}

	s = pterm.RemoveColorFromString(s)
	_, _ = fmt.Fprint(VerboseSecondaryOutput, s)
}

// Successf highlights a message as successful
func Successf(format string, a ...any) {
	Success(fmt.Sprintf(format, a...))
}

func Success(a ...any) {
	log(pterm.Style{pterm.FgGreen}, "✅ ", a...)
}

// Warnf highlights a message as a warning
func Warnf(format string, a ...any) {
	Warn(fmt.Sprintf(format, a...))
}

func Warn(a ...any) {
	log(pterm.Style{pterm.Bold, pterm.FgYellow}, "🔔 ", a...)
}

// Notef highlights a message as a note
func Notef(format string, a ...any) {
	Note(fmt.Sprintf(format, a...))
}

func Note(a ...any) {
	log(pterm.Style{pterm.FgLightYellow}, "", a...)
}

// Errorf highlights and formats a message as an error and
// shows the stack strace if the --verbose flag is active.
// Note: `err.Error()` is not printed, so if you want to include it
// in the output, make sure to include it in the other operands, for
// example:
//
//	Errorf(err, "Operation XYZ failed: %v", err)
func Errorf(err error, format string, a ...any) {
	Error(err, fmt.Sprintf(format, a...))
}

// Error highlights a message as an error and
// shows the stack strace if the --verbose flag is active.
// Note: `err.Error()` is only printed if there are no other operands
// than `err`. If you want to include it in the output, use `Errorf`
// instead (see the note there).
func Error(err error, a ...any) {
	// If no message is provided, print the error itself
	if len(a) == 0 {
		a = []any{err.Error()}
	}
	ErrorMsg(a...)

	type stackTracer interface {
		StackTrace() errors.StackTrace
	}
	var st stackTracer
	if viper.GetBool("verbose") &&
		errors.As(err, &st) {
		s := fmt.Sprintf("%+v", st.StackTrace())
		// Remove the leading newline to avoid an empty line between the
		// error message and the stack trace
		s = strings.TrimPrefix(s, "\n")
		Info(s)
	}
}

// ErrorMsgf highlights and formats a message as an error.
func ErrorMsgf(format string, a ...any) {
	ErrorMsg(fmt.Sprintf(format, a...))
}

// ErrorMsg highlights a message as an error.
func ErrorMsg(a ...any) {
	log(pterm.Style{pterm.Bold, pterm.FgRed}, "❌ ", a...)
}

// Infof outputs a regular user message without any highlighting
func Infof(format string, a ...any) {
	Info(fmt.Sprintf(format, a...))
}

func Info(a ...any) {
	log(pterm.Style{pterm.Fuzzy}, "", a...)
}

// Debugf outputs additional information when the --verbose flag is active
func Debugf(format string, a ...any) {
	Debug(fmt.Sprintf(format, a...))
}

func Debug(a ...any) {
	if viper.GetBool("verbose") {
		log(pterm.Style{pterm.Fuzzy}, "🔍 ", a...)
		return
	}

	// Secondary output catches full verbose log even
	// if it is not called in verbose mode
	logToSecondaryOutput(a...)
}

// Printf writes without any colors
func Printf(format string, a ...any) {
	Print(fmt.Sprintf(format, a...))
}

func Print(a ...any) {
	log(pterm.Style{pterm.FgDefault}, "", a...)
}

func Finding(a ...any) {
	log(pterm.Style{pterm.FgDefault}, "💥 ", a...)
}

func PlainStyle() bool {
	return viper.GetString("style") == "plain" || viper.GetBool("plain")
}
