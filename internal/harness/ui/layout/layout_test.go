package layout

import (
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/tui/ansi"
)

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

func TestResizeSequenceWithCursor(t *testing.T) {
	frame := ComputeFrame(140, 40, true)
	withCursor := ResizeSequenceWithCursor(frame, true, true)
	withoutCursor := ResizeSequenceWithCursor(frame, true, false)

	paneMove := ansi.Move(frame.ContentTop, frame.PaneXStart)

	if !contains(withCursor, paneMove) {
		t.Fatalf("ResizeSequenceWithCursor(..., true) missing pane move %q", paneMove)
	}

	if contains(withoutCursor, paneMove) {
		t.Fatalf("ResizeSequenceWithCursor(..., false) should not include pane move %q", paneMove)
	}
}

func TestSetupSequenceIncludesPaneMove(t *testing.T) {
	frame := ComputeFrame(140, 40, true)
	got := SetupSequence(frame, true)

	paneMove := ansi.Move(frame.ContentTop, frame.PaneXStart)
	if !contains(got, paneMove) {
		t.Fatalf("SetupSequence(...) missing pane move %q", paneMove)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
