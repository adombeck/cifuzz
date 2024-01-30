//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package executil

import (
	"os/exec"
	"syscall"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
)

// IsTerminatedExitErr returns true if the error is the expected error
// when the process was terminated via Cmd.TerminateProcessGroup and the
// process exited within the grace period.
//
// On Unix, multiple errors can happen when a SIGTERM is sent to the
// process group:
//   - The process can have exit code 143, which is the expected exit
//     code when a process receives a SIGTERM
//   - The process can have exit code -1 and the signal of the wait status
//     set to SIGTERM
//   - In case that the process doesn't handle the SIGTERM fast enough and
//     tries to write to the pipe which was already closed by
//     TerminateProcessGroup, the process has exit code -1 and the signal
//     of the wait status is set to SIGPIPE
func IsTerminatedExitErr(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	if exitErr.ExitCode() == 143 {
		return true
	}
	signal := exitErr.Sys().(syscall.WaitStatus).Signal()
	return signal == syscall.SIGTERM || signal == syscall.SIGPIPE
}

func (c *Cmd) TerminateProcessGroup() error {
	log.Infof("Sending SIGTERM to process group %d", c.Process.Pid)
	// We ignore errors here because the process group might not exist
	// anymore at this point.
	_ = syscall.Kill(-c.Process.Pid, syscall.SIGTERM) // note the minus sign

	// Close the write ends of any pipes to avoid that Wait blocks
	// until the command has finished printing output (which could be
	// indefinitely).
	c.closeDescriptors(c.CloseAfterWait)

	// Give the process group a few seconds to exit
	select {
	case <-time.After(processGroupTerminationGracePeriod):
		// The process group didn't exit within the grace period, so we
		// send it a SIGKILL now
		log.Infof("Sending SIGKILL to process group %d", c.Process.Pid)
		// We ignore errors here because the process group might not exist
		// anymore at this point.
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL) // note the minus sign
	case <-c.waitDone:
		// The process has already exited, nothing else to do here.
		// Note: This might leave other processes in the process group
		// running (which ignored the SIGTERM).
	}

	return nil
}

func (c *Cmd) prepareProcessGroupTermination() {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Make the child process use a new process group to be able to
	// terminate that process group on timeout.
	c.SysProcAttr.Setpgid = true
	// By forcing c.SysProcAttr.Pgid to be zero, we ensure that the
	// process ID of the child process is used as its process group ID
	// (see setpgid(2)).
	c.SysProcAttr.Pgid = 0
}
