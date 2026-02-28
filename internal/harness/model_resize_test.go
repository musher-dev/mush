//go:build unix

package harness

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
	"github.com/musher-dev/mush/internal/tui/ansi"
)

func TestClampTerminalSize(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "keeps valid size",
			width:      120,
			height:     40,
			wantWidth:  120,
			wantHeight: 40,
		},
		{
			name:       "clamps narrow width",
			width:      1,
			height:     40,
			wantWidth:  20,
			wantHeight: 40,
		},
		{
			name:       "clamps short height",
			width:      120,
			height:     1,
			wantWidth:  120,
			wantHeight: layout.TopBarHeight + 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := clampTerminalSize(tc.width, tc.height)
			if gotW != tc.wantWidth || gotH != tc.wantHeight {
				t.Fatalf("clampTerminalSize(%d, %d) = (%d, %d), want (%d, %d)",
					tc.width, tc.height, gotW, gotH, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

func TestPTYRowsForHeight(t *testing.T) {
	m := &RootModel{}

	if got := m.ptyRowsForHeight(layout.TopBarHeight + 20); got != 20 {
		t.Fatalf("ptyRowsForHeight(valid) = %d, want 20", got)
	}

	if got := m.ptyRowsForHeight(0); got != 1 {
		t.Fatalf("ptyRowsForHeight(clamped) = %d, want 1", got)
	}
}

func TestHandleResizeCallsResizable(t *testing.T) {
	type call struct {
		cols int
		rows int
	}

	var got call

	called := false

	mockExec := &mockResizable{
		onResize: func(rows, cols int) {
			called = true
			got = call{cols: cols, rows: rows}
		},
	}

	m := &RootModel{
		width:              80,
		height:             24,
		executors:          map[string]Executor{"claude": mockExec},
		supportedHarnesses: []string{"claude"},
	}

	m.handleResize(120, 40)

	if !called {
		t.Fatal("expected Resize to be called on executor")
	}

	if got.cols != 120 || got.rows != 40-layout.TopBarHeight {
		t.Fatalf("Resize called with cols=%d rows=%d, want cols=%d rows=%d",
			got.cols, got.rows, 120, 40-layout.TopBarHeight)
	}

	if m.width != 120 || m.height != 40 {
		t.Fatalf("model dimensions = (%d,%d), want (120,40)", m.width, m.height)
	}
}

func TestHandleResizeDoesNotForcePaneCursor(t *testing.T) {
	m := &RootModel{
		width:  80,
		height: 24,
	}

	output := captureStdout(t, func() {
		m.handleResize(100, 30)
	})

	frame := layout.ComputeFrame(100, 30, false)

	paneMove := ansi.Move(frame.ContentTop, frame.PaneXStart)
	if strings.Contains(output, paneMove) {
		t.Fatalf("handleResize output should not include pane move %q", paneMove)
	}
}

func TestRestoreLayoutAfterAltScreenDoesNotForcePaneCursor(t *testing.T) {
	m := &RootModel{
		width:             140,
		height:            40,
		lrMarginSupported: true,
	}

	output := captureStdout(t, func() {
		m.restoreLayoutAfterAltScreen()
	})

	frame := layout.ComputeFrame(140, 40, true)

	paneMove := ansi.Move(frame.ContentTop, frame.PaneXStart)
	if strings.Contains(output, paneMove) {
		t.Fatalf("restoreLayoutAfterAltScreen output should not include pane move %q", paneMove)
	}
}

// mockResizable is a test double implementing Executor + Resizable.
type mockResizable struct {
	onResize func(rows, cols int)
}

func (m *mockResizable) Setup(_ context.Context, _ *SetupOptions) error { return nil }
func (m *mockResizable) Execute(_ context.Context, _ *client.Job) (*ExecResult, error) {
	return &ExecResult{}, nil
}
func (m *mockResizable) Reset(_ context.Context) error { return nil }
func (m *mockResizable) Teardown()                     {}
func (m *mockResizable) Resize(rows, cols int) {
	if m.onResize != nil {
		m.onResize(rows, cols)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}

	os.Stdout = w
	done := make(chan string, 1)

	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	_ = r.Close()

	return <-done
}
