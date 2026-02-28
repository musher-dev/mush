package layout

import "testing"

func TestClampTerminalSize(t *testing.T) {
	w, h := ClampTerminalSize(1, 1)
	if w != 20 || h != TopBarHeight+1 {
		t.Fatalf("ClampTerminalSize(1,1) = (%d,%d)", w, h)
	}
}

func TestPtyRowsForHeight(t *testing.T) {
	if got := PtyRowsForHeight(TopBarHeight + 20); got != 20 {
		t.Fatalf("PtyRowsForHeight(valid) = %d, want 20", got)
	}

	if got := PtyRowsForHeight(0); got != 1 {
		t.Fatalf("PtyRowsForHeight(clamped) = %d, want 1", got)
	}
}

func TestComputeFrameShowsSidebarWhenWide(t *testing.T) {
	frame := ComputeFrame(140, 40, true)
	if !frame.SidebarVisible {
		t.Fatal("expected sidebar to be visible")
	}

	if frame.PaneWidth <= 0 || frame.PaneXStart <= 1 {
		t.Fatalf("invalid pane dimensions: %+v", frame)
	}
}
