package layout

import "github.com/musher-dev/mush/internal/tui/ansi"

const (
	// TopBarHeight is the number of lines reserved for the top status bar.
	TopBarHeight = 1

	defaultSidebarWidth = 36
	minSidebarWidth     = 24
	minPaneWidth        = 40
)

// Frame describes the terminal layout dimensions for the harness TUI.
type Frame struct {
	Width  int
	Height int

	ContentTop int

	SidebarVisible bool
	SidebarWidth   int

	PaneXStart int
	PaneWidth  int
}

// ClampTerminalSize enforces minimum terminal dimensions.
func ClampTerminalSize(width, height int) (clampedWidth, clampedHeight int) {
	if width < 20 {
		width = 20
	}

	minHeight := TopBarHeight + 1
	if height < minHeight {
		height = minHeight
	}

	return width, height
}

// ComputeFrame calculates the layout frame from terminal dimensions.
func ComputeFrame(width, height int, allowSidebar bool) Frame {
	width, height = ClampTerminalSize(width, height)
	frame := Frame{
		Width:      width,
		Height:     height,
		ContentTop: TopBarHeight + 1,
		PaneXStart: 1,
		PaneWidth:  width,
	}

	if !allowSidebar {
		return frame
	}

	if width < minPaneWidth+minSidebarWidth+1 {
		return frame
	}

	sidebar := defaultSidebarWidth

	remaining := width - minPaneWidth - 1
	if sidebar > remaining {
		sidebar = remaining
	}

	if sidebar < minSidebarWidth {
		return frame
	}

	frame.SidebarVisible = true
	frame.SidebarWidth = sidebar
	frame.PaneXStart = sidebar + 2 // one separator column
	frame.PaneWidth = width - sidebar - 1

	return frame
}

// PtyRowsForHeight returns the PTY row count for a given terminal height.
func PtyRowsForHeight(height int) int {
	rows := height - TopBarHeight
	if rows < 1 {
		return 1
	}

	return rows
}

// PtyRowsForFrame returns the PTY row count for a given layout frame.
func PtyRowsForFrame(frame Frame) int {
	return PtyRowsForHeight(frame.Height)
}

// SetupSequence returns the ANSI escape sequence for initial terminal setup.
func SetupSequence(frame Frame, useLRMargins bool) string {
	return ansi.ClearScreen + ResizeSequenceWithCursor(frame, useLRMargins, true)
}

// ResizeSequence returns the ANSI escape sequence for terminal resize.
func ResizeSequence(frame Frame, useLRMargins bool) string {
	return ResizeSequenceWithCursor(frame, useLRMargins, true)
}

// ResizeSequenceWithCursor returns the ANSI escape sequence for terminal resize.
// When moveCursor is false, pane constraints are applied without forcing cursor position.
func ResizeSequenceWithCursor(frame Frame, useLRMargins, moveCursor bool) string {
	seq := ansi.DisableLRMode
	if useLRMargins && frame.SidebarVisible {
		// DECSLRM moves the cursor to (1,1) and resets pending wrap state (per VT510 spec).
		// The explicit Move() at the end of this sequence neutralizes that side effect.
		seq = ansi.EnableLRMode + ansi.LRMargins(frame.PaneXStart, frame.Width)
	}

	seq += ansi.ScrollRegion(frame.ContentTop, frame.Height)
	if moveCursor {
		seq += ansi.Move(frame.ContentTop, frame.PaneXStart)
	}

	return seq
}
