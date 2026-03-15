//go:build unix

package harness

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/mattn/go-runewidth"

	"github.com/musher-dev/mush/internal/harness/ui/layout"
	statusui "github.com/musher-dev/mush/internal/harness/ui/status"
)

func (r *embeddedRuntime) draw() {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.drawLocked()
}

func (r *embeddedRuntime) drawLocked() {
	if r.screen == nil {
		return
	}

	r.renderTopBar()
	r.renderSidebar()
	r.renderViewport()
	r.screen.Show()
}

type styledSpan struct {
	text  string
	style tcell.Style
}

func (r *embeddedRuntime) renderTopBar() {
	barStyle := tcell.StyleDefault.Background(tnSurface).Foreground(tnText)

	for col := 0; col < r.width; col++ {
		r.screen.SetContent(col, 0, ' ', nil, barStyle)
	}

	snap := r.statusSnapshot()

	accentStyle := barStyle.Foreground(tnAccent).Bold(true)
	statusColor := statusTCellColor(snap.StatusLabel)
	statusStyle := barStyle.Foreground(statusColor).Bold(true)

	mode := "LIVE"
	modeStyle := barStyle.Foreground(tnSuccess)

	if !r.followTail {
		mode = fmt.Sprintf("SCROLL @%d", r.viewportTop)
		modeStyle = barStyle.Foreground(tnAccent)
	}

	spans := []styledSpan{
		{"MUSH", accentStyle},
		{"  Status: ", barStyle},
		{snap.StatusLabel, statusStyle},
		{"  Mode: ", barStyle},
		{mode, modeStyle},
		{fmt.Sprintf("  OK:%d Fail:%d", snap.Completed, snap.Failed), barStyle},
	}

	if snap.JobID != "" {
		spans = append(spans, styledSpan{"  Job: " + snap.JobID, barStyle})
	}

	if r.historyNotice != "" {
		spans = append(spans, styledSpan{"  " + r.historyNotice, barStyle.Foreground(tnWarning)})
	}

	right := "^C Int | ^Q Quit"

	leftWidth := 0
	for _, span := range spans {
		leftWidth += runewidth.StringWidth(span.text)
	}

	col := 0

	for _, span := range spans {
		for _, ch := range span.text {
			if col >= r.width {
				break
			}

			r.screen.SetContent(col, 0, ch, nil, span.style)
			col += runewidth.RuneWidth(ch)
		}
	}

	rightWidth := runewidth.StringWidth(right)
	rightStart := r.width - rightWidth

	if rightStart > leftWidth {
		hintCol := rightStart

		for _, ch := range right {
			if hintCol >= r.width {
				break
			}

			r.screen.SetContent(hintCol, 0, ch, nil, barStyle)
			hintCol += runewidth.RuneWidth(ch)
		}
	}
}

func statusTCellColor(label string) tcell.Color {
	switch label {
	case "Ready", "Connected":
		return tnSuccess
	case "Starting...", "Processing":
		return tnWarning
	case "Error":
		return tnError
	default:
		return tnText
	}
}

func (r *embeddedRuntime) renderSidebar() {
	if !r.frame.SidebarVisible {
		return
	}

	sideStyle := tcell.StyleDefault.Background(tnBorder).Foreground(tnText)
	borderStyle := tcell.StyleDefault.Background(tnSurface).Foreground(tnMuted)

	lines := r.sidebarLines(layout.PtyRowsForFrame(&r.frame))

	for row := 0; row < layout.PtyRowsForFrame(&r.frame); row++ {
		screenY := layout.TopBarHeight + row

		for col := 0; col < r.frame.SidebarWidth; col++ {
			r.screen.SetContent(col, screenY, ' ', nil, sideStyle)
		}

		line := ""
		if row < len(lines) {
			line = lines[row]
		}

		line = runewidth.Truncate(line, r.frame.SidebarWidth-1, "")
		line += strings.Repeat(" ", max(0, r.frame.SidebarWidth-runewidth.StringWidth(line)))

		col := 0
		for _, ch := range line {
			if col >= r.frame.SidebarWidth {
				break
			}

			r.screen.SetContent(col, screenY, ch, nil, sideStyle)
			col += runewidth.RuneWidth(ch)
		}

		r.screen.SetContent(r.frame.SidebarWidth, screenY, '│', nil, borderStyle)
	}
}

func (r *embeddedRuntime) sidebarLines(rows int) []string {
	snap := r.statusSnapshot()
	lines, targets := statusui.SidebarLines(&snap, rows)
	r.sidebarClickTargets = targets

	return lines
}

func (r *embeddedRuntime) renderViewport() {
	rows := layout.PtyRowsForFrame(&r.frame)
	paneX := r.frame.PaneXStart - 1
	paneY := r.frame.ContentTop - 1
	clearStyle := tcell.StyleDefault.Background(tnPTYBg).Foreground(tnText)

	for row := 0; row < rows; row++ {
		for col := 0; col < r.frame.ViewportWidth; col++ {
			r.screen.SetContent(paneX+col, paneY+row, ' ', nil, clearStyle)
		}
	}

	r.vt.Lock()
	defer r.vt.Unlock()

	for row := 0; row < rows; row++ {
		logicalRow := r.viewportTop + row
		if logicalRow < r.scrollback.Len() {
			r.renderGlyphRow(paneX, paneY+row, r.scrollback.Line(logicalRow))
			continue
		}

		vtRow := logicalRow - r.scrollback.Len()
		if vtRow < 0 || vtRow >= rows {
			continue
		}

		for col := 0; col < r.frame.ViewportWidth; col++ {
			glyph := r.vt.Cell(col, vtRow)
			r.screen.SetContent(paneX+col, paneY+row, glyphRune(glyph), nil, glyphStyle(glyph))
		}
	}

	r.renderScrollbar(paneY)

	// Alt-screen: child fully manages cursor display. Suppress everything.
	if r.isAltScreenActive() {
		r.screen.HideCursor()
		return
	}

	cursor := r.vt.Cursor()
	liveCursorRow := r.scrollback.Len() + cursor.Y
	inBounds := cursor.Y >= 0 && cursor.Y < rows &&
		cursor.X >= 0 && cursor.X < r.frame.ViewportWidth &&
		liveCursorRow >= r.viewportTop && liveCursorRow < r.viewportTop+rows

	if inBounds && r.vt.CursorVisible() {
		screenX := paneX + cursor.X
		screenY := paneY + liveCursorRow - r.viewportTop

		r.applySoftwareCursor(screenX, screenY)

		// Mouse-capture children (Ink/React TUIs) hide the cursor during
		// their render cycle and only set CursorVisible after repositioning
		// to the input element. Show the software cursor overlay but suppress
		// the blinking hardware cursor to avoid flicker at intermediate
		// positions during render transitions.
		if !r.childOwnsMouse() {
			r.screen.ShowCursor(screenX, screenY)
			return
		}
	}

	r.screen.HideCursor()
}

func (r *embeddedRuntime) renderGlyphRow(screenX, screenY int, cells []vt10x.Glyph) {
	for col := 0; col < r.frame.ViewportWidth; col++ {
		glyph := vt10x.Glyph{}
		if cells != nil && col < len(cells) {
			glyph = cells[col]
		}

		r.screen.SetContent(screenX+col, screenY, glyphRune(glyph), nil, glyphStyle(glyph))
	}
}

func (r *embeddedRuntime) applySoftwareCursor(screenX, screenY int) {
	content, style, _ := r.screen.Get(screenX, screenY)

	ch := ' '

	var combc []rune

	if content != "" {
		runes := []rune(content)

		ch = runes[0]
		if len(runes) > 1 {
			combc = append([]rune(nil), runes[1:]...)
		}
	}

	fg, bg, attrs := style.Decompose()
	reversed := tcell.StyleDefault.Foreground(bg).Background(fg).Attributes(attrs)
	r.screen.SetContent(screenX, screenY, ch, combc, reversed)
}

func (r *embeddedRuntime) renderScrollbar(paneY int) {
	if !r.frame.ScrollbarVisible {
		return
	}

	trackStyle := tcell.StyleDefault.Background(tnSurface).Foreground(tnMuted)
	thumbStyle := tcell.StyleDefault.Background(tnAccent).Foreground(tnSurface)
	thumbTop, thumbHeight := r.scrollbarThumb()
	scrollbarX := r.frame.ScrollbarXStart - 1

	for row := 0; row < r.visibleRows(); row++ {
		style := trackStyle
		ch := '│'

		if row >= thumbTop && row < thumbTop+thumbHeight {
			style = thumbStyle
			ch = '█'
		}

		r.screen.SetContent(scrollbarX, paneY+row, ch, nil, style)
	}
}

func glyphRune(glyph vt10x.Glyph) rune {
	if glyph.Char == 0 {
		return ' '
	}

	return glyph.Char
}

func glyphStyle(glyph vt10x.Glyph) tcell.Style {
	return tcell.StyleDefault.
		Foreground(vtColorToTCell(glyph.FG, true)).
		Background(vtColorToTCell(glyph.BG, false))
}

func vtColorToTCell(color vt10x.Color, isForeground bool) tcell.Color {
	if color == vt10x.DefaultFG {
		if isForeground {
			return tnText
		}

		return tnPTYBg
	}

	if color == vt10x.DefaultBG {
		if isForeground {
			return tnText
		}

		return tnPTYBg
	}

	if color < 16 {
		palette := []tcell.Color{
			tcell.ColorBlack, tcell.ColorMaroon, tcell.ColorGreen, tcell.ColorOlive,
			tcell.ColorNavy, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorSilver,
			tcell.ColorGray, tcell.ColorRed, tcell.ColorLime, tcell.ColorYellow,
			tcell.ColorBlue, tcell.ColorFuchsia, tcell.ColorAqua, tcell.ColorWhite,
		}

		return palette[int(color)]
	}

	if color < 256 {
		return tcell.PaletteColor(int(color))
	}

	rgb := int(color)
	red := (rgb >> 16) & 0xff
	green := (rgb >> 8) & 0xff
	blue := rgb & 0xff

	return tcell.NewRGBColor(int32(red), int32(green), int32(blue))
}
