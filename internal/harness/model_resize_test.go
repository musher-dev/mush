//go:build unix

package harness

import (
	"os"
	"testing"

	"github.com/creack/pty"
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
			wantHeight: StatusBarHeight + 1,
		},
	}

	for _, tc := range tests {
		tc := tc
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

	if got := m.ptyRowsForHeight(StatusBarHeight + 20); got != 20 {
		t.Fatalf("ptyRowsForHeight(valid) = %d, want 20", got)
	}

	if got := m.ptyRowsForHeight(0); got != 1 {
		t.Fatalf("ptyRowsForHeight(clamped) = %d, want 1", got)
	}
}

func TestHandleResizePropagatesPTYSize(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()

	type call struct {
		cols uint16
		rows uint16
	}
	var got call
	called := false

	m := &RootModel{
		width:  80,
		height: 24,
		ptmx:   r,
		setPTYSize: func(_ *os.File, ws *pty.Winsize) error {
			called = true
			got = call{cols: ws.Cols, rows: ws.Rows}
			return nil
		},
	}

	m.handleResize(120, 40)

	if !called {
		t.Fatal("expected setPTYSize to be called")
	}

	if got.cols != 120 || got.rows != uint16(40-StatusBarHeight) {
		t.Fatalf("setPTYSize called with cols=%d rows=%d, want cols=%d rows=%d",
			got.cols, got.rows, 120, 40-StatusBarHeight)
	}

	if m.width != 120 || m.height != 40 {
		t.Fatalf("model dimensions = (%d,%d), want (120,40)", m.width, m.height)
	}
}
