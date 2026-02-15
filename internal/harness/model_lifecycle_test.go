//go:build unix

package harness

import (
	"os"
	"testing"
	"time"

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
			Execution: &client.ExecutionConfig{AgentType: "claude"},
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
			Execution: &client.ExecutionConfig{AgentType: "claude"},
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
