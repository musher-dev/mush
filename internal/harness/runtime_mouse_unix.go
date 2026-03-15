//go:build unix

package harness

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"

	"github.com/musher-dev/mush/internal/harness/ui/layout"
)

const scrollLinesPerTick = 3

func (r *embeddedRuntime) handleMouse(ev *tcell.EventMouse) {
	mouseX, mouseY := ev.Position()
	buttons := ev.Buttons()

	if buttons == tcell.ButtonNone {
		r.uiMu.Lock()
		r.scrollbarDragging = false
		r.uiMu.Unlock()

		if r.childOwnsMouse() && r.isViewportCell(mouseX, mouseY) {
			r.writeInput(r.encodeMouseEvent(buttons, mouseX, mouseY))
		}

		return
	}

	if r.frame.SidebarVisible && mouseX < r.frame.SidebarWidth && mouseY >= layout.TopBarHeight && buttons == tcell.Button1 {
		sidebarRow := mouseY - layout.TopBarHeight
		r.handleSidebarClick(sidebarRow)

		return
	}

	if !r.isPaneRow(mouseY) {
		return
	}

	if r.childOwnsMouse() && r.isViewportCell(mouseX, mouseY) {
		if data := r.encodeMouseEvent(buttons, mouseX, mouseY); len(data) > 0 {
			r.writeInput(data)
		}

		return
	}

	switch buttons {
	case tcell.WheelUp:
		if !r.isScrollableCell(mouseX, mouseY) {
			return
		}

		r.scrollUp(scrollLinesPerTick)
	case tcell.WheelDown:
		if !r.isScrollableCell(mouseX, mouseY) {
			return
		}

		r.scrollDown(scrollLinesPerTick)
	case tcell.Button1:
		if r.handleScrollbarMouse(mouseX, mouseY) {
			return
		}
	}
}

func (r *embeddedRuntime) handleSidebarClick(row int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	for _, t := range r.sidebarClickTargets {
		if t.Row == row {
			r.sidebarExpanded[t.Section] = !r.sidebarExpanded[t.Section]
			r.drawLocked()

			return
		}
	}
}

func (r *embeddedRuntime) handleScrollbarMouse(mouseX, mouseY int) bool {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	if !r.isScrollbarCell(mouseX, mouseY) {
		if r.scrollbarDragging {
			r.moveScrollbarThumbLocked(mouseY)
			r.drawLocked()

			return true
		}

		return false
	}

	thumbTop, thumbHeight := r.scrollbarThumb()

	row := mouseY - (r.frame.ContentTop - 1)
	if row >= thumbTop && row < thumbTop+thumbHeight {
		r.scrollbarDragging = true
		r.scrollbarDragY = row - thumbTop

		return true
	}

	r.scrollbarDragging = false
	if row < thumbTop {
		r.viewportTop -= max(r.visibleRows()-1, 1)
		r.followTail = false
	} else {
		r.viewportTop += max(r.visibleRows()-1, 1)
	}

	r.clampViewportLocked()
	r.drawLocked()

	return true
}

func (r *embeddedRuntime) moveScrollbarThumbLocked(y int) {
	trackRows := r.visibleRows()
	thumbTop, thumbHeight := r.scrollbarThumb()
	_ = thumbTop

	row := y - (r.frame.ContentTop - 1) - r.scrollbarDragY
	if row < 0 {
		row = 0
	}

	maxThumbTop := trackRows - thumbHeight
	if maxThumbTop < 0 {
		maxThumbTop = 0
	}

	if row > maxThumbTop {
		row = maxThumbTop
	}

	maxTop := r.maxViewportTop()
	if maxThumbTop == 0 || maxTop == 0 {
		r.endScrollLocked()
		return
	}

	r.followTail = false
	r.viewportTop = row * maxTop / maxThumbTop
	r.clampViewportLocked()
}

func (r *embeddedRuntime) scrollbarThumb() (top, height int) {
	trackRows := r.visibleRows()
	if trackRows <= 0 {
		return 0, 0
	}

	totalRows := r.totalRows()
	if totalRows <= 0 || totalRows <= trackRows {
		return 0, trackRows
	}

	height = max(1, trackRows*trackRows/totalRows)
	if height > trackRows {
		height = trackRows
	}

	maxTop := r.maxViewportTop()
	if maxTop == 0 {
		return 0, height
	}

	top = r.viewportTop * (trackRows - height) / maxTop

	return top, height
}

func (r *embeddedRuntime) isPaneRow(y int) bool {
	return y >= r.frame.ContentTop-1 && y < r.frame.ContentTop-1+r.visibleRows()
}

func (r *embeddedRuntime) isViewportCell(x, y int) bool {
	return r.isPaneRow(y) && x >= r.frame.PaneXStart-1 && x < r.frame.PaneXStart-1+r.frame.ViewportWidth
}

func (r *embeddedRuntime) isScrollbarCell(x, y int) bool {
	if !r.frame.ScrollbarVisible {
		return false
	}

	return r.isPaneRow(y) && x == r.frame.ScrollbarXStart-1
}

func (r *embeddedRuntime) isScrollableCell(x, y int) bool {
	return r.isViewportCell(x, y) || r.isScrollbarCell(x, y)
}

func (r *embeddedRuntime) encodeMouseEvent(buttons tcell.ButtonMask, mouseX, mouseY int) []byte {
	if !r.isViewportCell(mouseX, mouseY) {
		return nil
	}

	col := mouseX - (r.frame.PaneXStart - 1) + 1
	row := mouseY - (r.frame.ContentTop - 1) + 1

	buttonCode, release, ok := mouseButtonCode(buttons)
	if !ok {
		return nil
	}

	if r.vt.Mode()&vt10x.ModeMouseSgr != 0 {
		suffix := "M"
		if release {
			suffix = "m"
		}

		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%s", buttonCode, col, row, suffix))
	}

	if release {
		buttonCode = 3
	}

	return []byte{0x1b, '[', 'M', byte(buttonCode + 32), byte(col + 32), byte(row + 32)}
}

func mouseButtonCode(buttons tcell.ButtonMask) (code int, release, ok bool) {
	switch buttons {
	case tcell.Button1:
		return 0, false, true
	case tcell.Button2:
		return 1, false, true
	case tcell.Button3:
		return 2, false, true
	case tcell.ButtonNone:
		return 3, true, true
	case tcell.WheelUp:
		return 64, false, true
	case tcell.WheelDown:
		return 65, false, true
	}

	if buttons&tcell.Button1 != 0 {
		return 32, false, true
	}

	return 0, false, false
}
