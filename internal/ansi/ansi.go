package ansi

import "strings"

// Strip removes ANSI escape sequences from a string.
func Strip(s string) string {
	var b strings.Builder
	inEscape := false
	var escBuf []rune
	for _, r := range s {
		if !inEscape {
			if r == '\x1b' {
				inEscape = true
				escBuf = escBuf[:0]
				escBuf = append(escBuf, r)
				continue
			}
			b.WriteRune(r)
			continue
		}

		escBuf = append(escBuf, r)
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			inEscape = false
			escBuf = escBuf[:0]
		}
		continue
	}
	if inEscape {
		for _, r := range escBuf {
			b.WriteRune(r)
		}
	}
	return b.String()
}
