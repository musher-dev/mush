package ansi

import "fmt"

// ANSI escape sequence constants for terminal control.
const (
	ClearScreen   = "\x1b[2J"
	MoveTo        = "\x1b[%d;%dH" // row;col (1-indexed)
	SaveCursor    = "\x1b7"       // DECSC — safe even when DECLRMM (mode 69) is active
	RestoreCursor = "\x1b8"       // DECRC
	SetScrollRgn  = "\x1b[%d;%dr" // top;bottom
	SetLRMargins  = "\x1b[%d;%ds" // left;right (DECSLRM)
	ResetScroll   = "\x1b[r"
	Reset         = "\x1b[0m"
	ShowCursor    = "\x1b[?25h"
	HideCursor    = "\x1b[?25l"
	ClearLine     = "\x1b[2K"
	EnableLRMode  = "\x1b[?69h"
	DisableLRMode = "\x1b[?69l"
)

// Move returns an ANSI cursor movement sequence.
func Move(row, col int) string {
	return fmt.Sprintf(MoveTo, row, col)
}

// ScrollRegion returns an ANSI scroll region sequence.
func ScrollRegion(top, bottom int) string {
	return fmt.Sprintf(SetScrollRgn, top, bottom)
}

// LRMargins returns an ANSI DECSLRM left/right margin sequence.
func LRMargins(left, right int) string {
	return fmt.Sprintf(SetLRMargins, left, right)
}
