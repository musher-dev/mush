//go:build unix

package harnesstype

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
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

// StartInteractiveProcess starts a PTY-backed process and wires output and exit handling.
func StartInteractiveProcess(
	cmd *exec.Cmd,
	opts *SetupOptions,
	onExit func(),
) (ptmx *os.File, pgid int, waitDoneCh chan struct{}, err error) {
	ptmx, err = pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		return nil, 0, nil, fmt.Errorf("start interactive pty: %w", err)
	}

	pgid = processGroupID(cmd)
	waitDoneCh = make(chan struct{})

	go streamInteractiveOutput(ptmx, opts)
	go waitInteractiveProcess(cmd, waitDoneCh, onExit)

	return ptmx, pgid, waitDoneCh, nil
}

func processGroupID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return 0
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return 0
	}

	return pgid
}

func streamInteractiveOutput(ptmx *os.File, opts *SetupOptions) {
	buf := make([]byte, 4096)
	for {
		n, readErr := ptmx.Read(buf)
		if n > 0 {
			if opts.TermWriter != nil {
				_, _ = opts.TermWriter.Write(buf[:n])
			}

			if opts.OnOutput != nil {
				opts.OnOutput(buf[:n])
			}
		}

		if readErr != nil {
			return
		}
	}
}

func waitInteractiveProcess(cmd *exec.Cmd, waitDoneCh chan struct{}, onExit func()) {
	_ = cmd.Wait()

	close(waitDoneCh)

	if onExit != nil {
		onExit()
	}
}
