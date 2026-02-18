//go:build unix

package harness

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/musher-dev/mush/internal/client"
)

func TestHandleCtrlCExitsImmediatelyWithoutActiveClaudeJob(t *testing.T) {
	m := &RootModel{
		done: make(chan struct{}),
	}

	if !m.handleCtrlC() {
		t.Fatal("handleCtrlC() = false, want true when no active Claude job")
	}

	select {
	case <-m.done:
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestHandleCtrlCFirstPressInterruptsClaude(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()

	now := time.Unix(1000, 0)
	m := &RootModel{
		done:            make(chan struct{}),
		ptmx:            w,
		now:             func() time.Time { return now },
		ctrlCExitWindow: 2 * time.Second,
		currentJob: &client.Job{
			Execution: &client.ExecutionConfig{HarnessType: "claude"},
		},
	}

	if m.handleCtrlC() {
		t.Fatal("first handleCtrlC() = true, want false to keep running")
	}

	b := make([]byte, 1)
	if _, err := r.Read(b); err != nil {
		t.Fatalf("read interrupt byte failed: %v", err)
	}
	if b[0] != ctrlC {
		t.Fatalf("interrupt byte = %d, want %d", b[0], ctrlC)
	}

	select {
	case <-m.done:
		t.Fatal("done channel should remain open after first Ctrl+C")
	default:
	}
}

func TestHandleCtrlCSecondPressExitsWithinWindow(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()

	base := time.Unix(2000, 0)
	current := base
	m := &RootModel{
		done:            make(chan struct{}),
		ptmx:            w,
		now:             func() time.Time { return current },
		ctrlCExitWindow: 2 * time.Second,
		currentJob: &client.Job{
			Execution: &client.ExecutionConfig{HarnessType: "claude"},
		},
	}

	if m.handleCtrlC() {
		t.Fatal("first handleCtrlC() = true, want false")
	}

	current = base.Add(1500 * time.Millisecond)
	if !m.handleCtrlC() {
		t.Fatal("second handleCtrlC() = false, want true within exit window")
	}

	select {
	case <-m.done:
	default:
		t.Fatal("expected done channel to be closed after second Ctrl+C")
	}
}

func TestStartPTYDoesNotSetSetpgid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()

	var gotCmd *exec.Cmd
	m := &RootModel{
		ctx:      t.Context(),
		width:    80,
		height:   24,
		done:     make(chan struct{}),
		ptyReady: make(chan *os.File, 1),
		startPTYWithSize: func(cmd *exec.Cmd, _ *pty.Winsize) (*os.File, error) {
			gotCmd = cmd
			return r, nil
		},
	}
	defer m.closePTY()

	if err := m.startPTY(); err != nil {
		t.Fatalf("startPTY() error = %v", err)
	}

	if gotCmd == nil {
		t.Fatal("expected startPTYWithSize to receive command")
	}
	if gotCmd.SysProcAttr != nil && gotCmd.SysProcAttr.Setpgid {
		t.Fatal("Setpgid should not be set when launching via pty.StartWithSize")
	}
}

func TestSendSignalPrefersProcessGroup(t *testing.T) {
	var targets []int
	m := &RootModel{
		killProcess: func(target int, _ syscall.Signal) error {
			targets = append(targets, target)
			return nil
		},
	}

	m.sendSignal(1234, 4321, syscall.SIGTERM)

	if len(targets) != 1 || targets[0] != -4321 {
		t.Fatalf("sendSignal targets = %v, want [-4321]", targets)
	}
}

func TestSendSignalFallsBackToPIDWhenGroupSignalFails(t *testing.T) {
	var targets []int
	m := &RootModel{
		killProcess: func(target int, _ syscall.Signal) error {
			targets = append(targets, target)
			if target < 0 {
				return errors.New("group signal failed")
			}
			return nil
		},
	}

	m.sendSignal(1234, 4321, syscall.SIGTERM)

	if len(targets) != 2 || targets[0] != -4321 || targets[1] != 1234 {
		t.Fatalf("sendSignal fallback targets = %v, want [-4321 1234]", targets)
	}
}

func TestAnnotateStartPTYErrorAddsHintForEPERM(t *testing.T) {
	baseErr := &os.PathError{Op: "fork/exec", Path: "/tmp/claude", Err: syscall.EPERM}
	err := annotateStartPTYError(baseErr, "/tmp/claude")

	if !errors.Is(err, syscall.EPERM) {
		t.Fatalf("annotated error should preserve EPERM: %v", err)
	}
	if got := err.Error(); !containsAll(got, []string{"EPERM during PTY start", "noexec", "quarantine"}) {
		t.Fatalf("annotated error missing hints: %q", got)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
