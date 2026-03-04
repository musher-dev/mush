//go:build unix

package harness

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func stopInteractiveProcess(cmd *exec.Cmd, ptmx *os.File, pgid int, waitDoneCh chan struct{}) {
	if ptmx != nil {
		_ = ptmx.Close()
	}

	if cmd == nil || cmd.Process == nil {
		return
	}

	sendSignal(cmd.Process.Pid, pgid, syscall.SIGTERM)

	select {
	case <-waitDoneCh:
	case <-time.After(2 * time.Second):
		sendSignal(cmd.Process.Pid, pgid, syscall.SIGKILL)

		if waitDoneCh != nil {
			select {
			case <-waitDoneCh:
			case <-time.After(2 * time.Second):
			}
		}
	}
}
