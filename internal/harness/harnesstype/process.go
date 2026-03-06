//go:build unix

package harnesstype

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// SendSignal sends a signal to a process group first, falling back to the PID.
func SendSignal(pid, pgid int, sig syscall.Signal) {
	if pgid > 0 {
		if err := syscall.Kill(-pgid, sig); err == nil || errors.Is(err, syscall.ESRCH) {
			return
		}
	}

	if pid <= 0 {
		return
	}

	_ = syscall.Kill(pid, sig)
}

// StopInteractiveProcess gracefully terminates an interactive PTY process.
func StopInteractiveProcess(cmd *exec.Cmd, ptmx *os.File, pgid int, waitDoneCh chan struct{}) {
	if ptmx != nil {
		_ = ptmx.Close()
	}

	if cmd == nil || cmd.Process == nil {
		return
	}

	SendSignal(cmd.Process.Pid, pgid, syscall.SIGTERM)

	select {
	case <-waitDoneCh:
	case <-time.After(2 * time.Second):
		SendSignal(cmd.Process.Pid, pgid, syscall.SIGKILL)

		if waitDoneCh != nil {
			select {
			case <-waitDoneCh:
			case <-time.After(2 * time.Second):
			}
		}
	}
}
