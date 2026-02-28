package render

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/musher-dev/mush/internal/ansi"
)

// VisibleLength returns the visible cell width of a string, excluding ANSI
// escape sequences and accounting for wide (CJK) and zero-width characters.
func VisibleLength(value string) int {
	return runewidth.StringWidth(ansi.Strip(value))
}

// PadRightVisible appends spaces until the string reaches width visible cells.
func PadRightVisible(value string, width int) string {
	padding := width - VisibleLength(value)
	if padding <= 0 {
		return value
	}

	return value + strings.Repeat(" ", padding)
}
