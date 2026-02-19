//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func TestHandleCtrlCExitsImmediatelyWithoutActiveClaudeJob(t *testing.T) {
	m := &RootModel{
		done:      make(chan struct{}),
		executors: make(map[string]Executor),
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

	// Create a mock claude executor that captures input.
	ce := &mockInputReceiver{w: w}

	m := &RootModel{
		done:               make(chan struct{}),
		executors:          map[string]Executor{"claude": ce},
		supportedHarnesses: []string{"claude"},
		now:                func() time.Time { return now },
		ctrlCExitWindow:    2 * time.Second,
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

	ce := &mockInputReceiver{w: w}

	m := &RootModel{
		done:               make(chan struct{}),
		executors:          map[string]Executor{"claude": ce},
		supportedHarnesses: []string{"claude"},
		now:                func() time.Time { return current },
		ctrlCExitWindow:    2 * time.Second,
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

// mockInputReceiver is a test double for InputReceiver.
type mockInputReceiver struct {
	w *os.File
}

func (m *mockInputReceiver) Setup(_ context.Context, _ *SetupOptions) error { return nil }
func (m *mockInputReceiver) Execute(_ context.Context, _ *client.Job) (*ExecResult, error) {
	return &ExecResult{}, nil
}
func (m *mockInputReceiver) Reset(_ context.Context) error { return nil }
func (m *mockInputReceiver) Teardown()                     {}

func (m *mockInputReceiver) WriteInput(p []byte) (int, error) {
	n, err := m.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("write input: %w", err)
	}

	return n, nil
}
