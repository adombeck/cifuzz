package executil

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

// IsTerminatedExitErr returns true if the wait status is the
// wait status that is expected when the process was terminated via
// Cmd.TerminateProcessGroup.
//
// On Windows, processes which are terminated via taskkill exit with
// exit code 1.
func IsTerminatedExitErr(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.Sys().(syscall.WaitStatus).ExitCode == 1
}

// TerminateProcessGroup uses the taskkill command with the /t and /f
// parameters to forcefully terminate the process of the command and
// any child processes started by it.
// See https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/taskkill
//
// Important: Note that on Windows, when using StdoutTeePipe or
// StderrTeePipe, TerminateProcessGroup doesn't cause the process
// to exit if the pipes were not closed yet. In that case, Wait will
// block until the pipes are closed.
func (c *Cmd) TerminateProcessGroup() error {
	defer c.closeDescriptors(c.CloseAfterWait)

	// Based on https://stackoverflow.com/a/44551450/2804197
	// Original author: https://stackoverflow.com/users/301049/rots
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid))
	kill.Stderr = os.Stderr
	kill.Stdout = os.Stderr
	err := kill.Run()
	// taskkill can fail e.g. because the process has already been terminated.
	// We only report non-ExitErrors.
	if _, isExitErr := err.(*exec.ExitError); isExitErr {
		return nil
	}
	return errors.WithStack(err)
}

func (c *Cmd) prepareProcessGroupTermination() {
	// Nothing to prepare on Windows
}
