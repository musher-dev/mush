package render

import "strings"

// VisibleLength returns the visible length of a string, excluding ANSI codes.
func VisibleLength(value string) int {
	length := 0
	inEscape := false

	for _, r := range value {
		if r == '\x1b' {
			inEscape = true
			continue
		}

		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}

			continue
		}

		length++
	}

	return length
}

// PadRightVisible appends spaces until the string reaches width visible cells.
func PadRightVisible(value string, width int) string {
	padding := width - VisibleLength(value)
	if padding <= 0 {
		return value
	}

	return value + strings.Repeat(" ", padding)
}
