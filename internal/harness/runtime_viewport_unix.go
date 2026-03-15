//go:build unix

package harness

import (
	"github.com/hinshun/vt10x"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
)

func (r *embeddedRuntime) handleResize(width, height int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	width, height = clampTerminalSize(width, height)
	oldViewportWidth := r.frame.ViewportWidth

	r.width, r.height = width, height
	r.frame = layout.ComputeFrame(width, height, true)
	r.vt.Resize(r.frame.ViewportWidth, layout.PtyRowsForFrame(&r.frame))

	rows := layout.PtyRowsForFrame(&r.frame)
	for _, executor := range r.executors {
		if rs, ok := executor.(harnesstype.Resizable); ok {
			rs.Resize(rows, r.frame.ViewportWidth)
		}
	}

	if oldViewportWidth != 0 && oldViewportWidth != r.frame.ViewportWidth {
		r.invalidateHistoryLocked("History reset after resize")
	}

	r.clampViewportLocked()
	r.screen.Clear()
	r.drawLocked()
}

func (r *embeddedRuntime) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.historyNotice = ""
	r.captureScrolledLines(p)
	r.syncMouseCaptureLocked()
	r.drawLocked()

	return len(p), nil
}

// captureScrolledLines snapshots visible rows before vt.Write, writes to vt,
// then detects which top rows scrolled off and pushes them to scrollback.
func (r *embeddedRuntime) captureScrolledLines(p []byte) {
	if r.vt.Mode()&vt10x.ModeAltScreen != 0 {
		_, _ = r.vt.Write(p)
		if r.followTail {
			r.viewportTop = r.maxViewportTop()
		}

		return
	}

	rows := layout.PtyRowsForFrame(&r.frame)
	cols := r.frame.ViewportWidth
	oldHistoryLen := r.scrollback.Len()
	before := r.snapshotVisibleRows(rows, cols)

	_, _ = r.vt.Write(p)

	after := r.snapshotVisibleRows(rows, cols)
	scrolledOff := 0

	for shift := 1; shift < rows; shift++ {
		if glyphRowsEqual(before[shift:], after[:rows-shift], cols) {
			scrolledOff = shift
		}
	}

	for i := 0; i < scrolledOff; i++ {
		r.scrollback.Push(before[i])
	}

	if scrolledOff > 0 {
		if r.followTail {
			r.viewportTop = r.maxViewportTop()
		} else if r.viewportTop >= oldHistoryLen {
			r.viewportTop += scrolledOff
		}
	}

	r.clampViewportLocked()
}

func (r *embeddedRuntime) snapshotVisibleRows(rows, cols int) [][]vt10x.Glyph {
	lines := make([][]vt10x.Glyph, rows)

	r.vt.Lock()
	defer r.vt.Unlock()

	for row := 0; row < rows; row++ {
		line := make([]vt10x.Glyph, cols)
		for col := 0; col < cols; col++ {
			line[col] = r.vt.Cell(col, row)
		}

		lines[row] = line
	}

	return lines
}

func glyphRowsEqual(left, right [][]vt10x.Glyph, cols int) bool {
	if len(left) != len(right) {
		return false
	}

	for row := range left {
		for col := 0; col < cols && col < len(left[row]) && col < len(right[row]); col++ {
			if left[row][col] != right[row][col] {
				return false
			}
		}
	}

	return true
}

func (r *embeddedRuntime) scrollUp(n int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.followTail = false
	r.viewportTop -= n
	r.clampViewportLocked()
	r.drawLocked()
}

func (r *embeddedRuntime) scrollDown(n int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.viewportTop += n
	r.clampViewportLocked()
	r.drawLocked()
}

func (r *embeddedRuntime) scrollToTop() {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.followTail = false
	r.viewportTop = 0
	r.clampViewportLocked()
	r.drawLocked()
}

func (r *embeddedRuntime) scrollToBottom() {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.endScrollLocked()
	r.drawLocked()
}

func (r *embeddedRuntime) endScrollLocked() {
	r.followTail = true
	r.viewportTop = r.maxViewportTop()
}

func (r *embeddedRuntime) invalidateHistoryLocked(notice string) {
	r.scrollback.Clear()
	r.historyNotice = notice
	r.followTail = true
	r.viewportTop = 0
}

func (r *embeddedRuntime) visibleRows() int {
	return layout.PtyRowsForFrame(&r.frame)
}

func (r *embeddedRuntime) totalRows() int {
	return r.scrollback.Len() + r.visibleRows()
}

func (r *embeddedRuntime) maxViewportTop() int {
	maxTop := r.totalRows() - r.visibleRows()
	if maxTop < 0 {
		return 0
	}

	return maxTop
}

func (r *embeddedRuntime) clampViewportLocked() {
	maxTop := r.maxViewportTop()
	if r.followTail {
		r.viewportTop = maxTop
		return
	}

	if r.viewportTop < 0 {
		r.viewportTop = 0
	}

	if r.viewportTop >= maxTop {
		r.viewportTop = maxTop
		r.followTail = true
	}
}

func (r *embeddedRuntime) isAltScreenActive() bool {
	return r.vt.Mode()&vt10x.ModeAltScreen != 0
}

func (r *embeddedRuntime) childOwnsMouse() bool {
	return r.isAltScreenActive() || r.vt.Mode()&vt10x.ModeMouseMask != 0
}
