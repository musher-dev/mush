//go:build unix

package harness

import (
	"bytes"
	"strconv"
)

// sidebarFilter is a stream filter for PTY output when the sidebar is active.
// It rewrites bare CSI s (SCOSC) → ESC 7 (DECSC), bare CSI u (SCORC) → ESC 8
// (DECRC), bare CSI r → CSI <top>;<bottom> r (preserve scroll region), and
// drops CSI ?69l (disable LR mode) and parameterized CSI Ps;Ps s (DECSLRM).
//
// The filter uses a tail buffer of up to 8 bytes to handle sequences split
// across Write chunk boundaries. If accumulated bytes exceed the buffer,
// they are flushed through (the onWrite callback still catches the event).
type sidebarFilter struct {
	active   func() bool                 // true when sidebar is enabled (DECLRMM active)
	paneDims func() (int, int, int, int) // returns (contentTop, scrollBottom, paneXStart, termWidth)
	tail     [16]byte                    // held bytes from previous chunk (cursor seqs can reach ~12 bytes)
	tailN    int                         // number of valid bytes in tail (0..16)
}

// rewrite processes a chunk of PTY output. When active, it rewrites or drops
// sequences that would break the sidebar layout. Designed for the hot path:
// if no ESC byte is present and no tail is held, the input slice is returned
// directly (zero allocation).
func (sf *sidebarFilter) rewrite(p []byte) []byte {
	if !sf.active() {
		// Not active — flush any held tail and pass through.
		if sf.tailN > 0 {
			out := make([]byte, sf.tailN+len(p))
			copy(out, sf.tail[:sf.tailN])
			copy(out[sf.tailN:], p)
			sf.tailN = 0

			return out
		}

		return p
	}

	// Fast path: no held tail and no ESC in chunk.
	if sf.tailN == 0 && !containsByte(p, 0x1b) {
		return p
	}

	// Prepend any held tail bytes.
	var src []byte
	if sf.tailN > 0 {
		src = make([]byte, sf.tailN+len(p))
		copy(src, sf.tail[:sf.tailN])
		copy(src[sf.tailN:], p)
		sf.tailN = 0
	} else {
		src = p
	}

	// Lazy allocation: out stays nil until we encounter a rewrite, drop, or
	// tail hold. This avoids allocating for the common case where the chunk
	// contains ESC sequences (colors, cursor moves) but nothing we filter.
	var out []byte

	i := 0

	for i < len(src) {
		if src[i] != 0x1b {
			if out != nil {
				out = append(out, src[i])
			}

			i++

			continue
		}

		// We have ESC at position i.
		if i+1 >= len(src) {
			// ESC at end of chunk — hold it.
			sf.tail[0] = 0x1b
			sf.tailN = 1

			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			break
		}

		if src[i+1] != '[' {
			// ESC followed by non-'[' — not a CSI, pass through.
			if out != nil {
				out = append(out, src[i], src[i+1])
			}

			i += 2

			continue
		}

		// ESC [ at positions i, i+1.
		if i+2 >= len(src) {
			// ESC [ at end of chunk — hold both bytes.
			sf.tail[0] = 0x1b
			sf.tail[1] = '['
			sf.tailN = 2

			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			break
		}

		third := src[i+2]

		switch {
		case third == 's':
			// Bare CSI s → ESC 7 (DECSC).
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			out = append(out, 0x1b, '7')
			i += 3

		case third == 'u':
			// Bare CSI u → ESC 8 (DECRC).
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			out = append(out, 0x1b, '8')
			i += 3

		case third == 'r':
			// Bare CSI r → CSI <top>;<bottom> r (preserve scroll region).
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			top, bottom, _, _ := sf.paneDims()

			out = append(out, "\x1b["...)
			out = strconv.AppendInt(out, int64(top), 10)
			out = append(out, ';')
			out = strconv.AppendInt(out, int64(bottom), 10)
			out = append(out, 'r')
			i += 3

		case third == '?':
			// CSI ? ... — accumulate to find final byte.
			action, seqLen := sf.handleCSIQuestion(src[i:])
			if seqLen == 0 {
				// Incomplete — hold remaining bytes in tail.
				held := src[i:]
				if len(held) > len(sf.tail) {
					// Overflow — flush through.
					if out != nil {
						out = append(out, held...)
					}
				} else {
					copy(sf.tail[:], held)
					sf.tailN = len(held)

					if out == nil {
						out = make([]byte, i, len(src))
						copy(out, src[:i])
					}
				}

				i = len(src)

				break
			}

			if action == filterDrop {
				if out == nil {
					out = make([]byte, i, len(src))
					copy(out, src[:i])
				}
				// Drop — don't append anything.
			} else if out != nil {
				out = append(out, src[i:i+seqLen]...)
			}

			i += seqLen

		case (third >= '0' && third <= '9') || third == ';':
			// Parameterized CSI — accumulate to find final byte.
			action, seqLen, rewritten := sf.handleCSIParams(src[i:])
			if seqLen == 0 {
				// Incomplete — hold remaining bytes in tail.
				held := src[i:]
				if len(held) > len(sf.tail) {
					// Overflow — flush through.
					if out != nil {
						out = append(out, held...)
					}
				} else {
					copy(sf.tail[:], held)
					sf.tailN = len(held)

					if out == nil {
						out = make([]byte, i, len(src))
						copy(out, src[:i])
					}
				}

				i = len(src)

				break
			}

			switch action {
			case filterDrop:
				if out == nil {
					out = make([]byte, i, len(src))
					copy(out, src[:i])
				}
			case filterRewrite:
				if out == nil {
					out = make([]byte, i, len(src))
					copy(out, src[:i])
				}

				out = append(out, rewritten...)
			default:
				if out != nil {
					out = append(out, src[i:i+seqLen]...)
				}
			}

			i += seqLen

		case third == 'H' || third == 'f':
			// Bare CUP (CSI H / CSI f) — translate default (1,1).
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			top, _, xStart, _ := sf.paneDims()
			out = append(out, buildCUP(top, xStart, third)...)
			i += 3

		case third == 'G' || third == '`':
			// Bare CHA / HPA — translate default col 1.
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			_, _, xStart, _ := sf.paneDims()
			out = append(out, buildSingleParam(xStart, third)...)
			i += 3

		case third == 'd':
			// Bare VPA — translate default row 1.
			if out == nil {
				out = make([]byte, i, len(src))
				copy(out, src[:i])
			}

			top, _, _, _ := sf.paneDims()
			out = append(out, buildSingleParam(top, third)...)
			i += 3

		case third == '!':
			// CSI ! p (soft reset) — pass through.
			if out != nil {
				out = append(out, src[i], src[i+1])
			}

			i += 2

		default:
			// Other bare CSI final — pass through.
			if out != nil {
				out = append(out, src[i], src[i+1])
			}

			i += 2
		}
	}

	if out != nil {
		return out
	}

	return src
}

type filterAction int

const (
	filterPass filterAction = iota
	filterDrop
	filterRewrite
)

// handleCSIQuestion processes CSI ? ... sequences starting at seq[0]=ESC.
// Returns the action and total sequence length. seqLen=0 means incomplete.
func (sf *sidebarFilter) handleCSIQuestion(seq []byte) (action filterAction, seqLen int) {
	// seq starts with ESC [ ?
	// Find the final byte (0x40-0x7E).
	for j := 3; j < len(seq); j++ {
		b := seq[j]
		if b >= 0x40 && b <= 0x7e {
			// Final byte found. Check for CSI ?69l without string allocation.
			if b == 'l' && j-3 == 2 && seq[3] == '6' && seq[4] == '9' {
				return filterDrop, j + 1
			}

			return filterPass, j + 1
		}
	}

	return filterPass, 0 // incomplete
}

// handleCSIParams processes CSI <digits/;> ... sequences starting at seq[0]=ESC.
// Returns the action, total sequence length, and optional rewritten bytes.
// seqLen=0 means incomplete. rewritten is non-nil only when action is filterRewrite.
func (sf *sidebarFilter) handleCSIParams(seq []byte) (action filterAction, seqLen int, rewritten []byte) {
	// seq starts with ESC [ <digit or ;>
	// Find the final byte (0x40-0x7E).
	for j := 2; j < len(seq); j++ {
		b := seq[j]
		if b >= 0x40 && b <= 0x7e {
			// Final byte found. Check for DECSLRM without string allocation.
			if b == 's' && isDECSLRMBytes(seq[2:j]) {
				return filterDrop, j + 1, nil
			}

			// Cursor-addressing sequences — translate coordinates.
			params := seq[2:j]

			switch b {
			case 'H', 'f': // CUP — row;col
				top, bottom, xStart, termW := sf.paneDims()
				row, col := parseTwoParams(params)
				row = clampInt(row+top-1, top, bottom)
				col = clampInt(col+xStart-1, xStart, termW)

				return filterRewrite, j + 1, buildCUP(row, col, b)

			case 'G', '`': // CHA / HPA — col
				_, _, xStart, termW := sf.paneDims()
				col := parseOneParam(params)
				col = clampInt(col+xStart-1, xStart, termW)

				return filterRewrite, j + 1, buildSingleParam(col, b)

			case 'd': // VPA — row
				top, bottom, _, _ := sf.paneDims()
				row := parseOneParam(params)
				row = clampInt(row+top-1, top, bottom)

				return filterRewrite, j + 1, buildSingleParam(row, b)

			case 'K': // EL — Erase in Line
				mode := parseParamBytes(params)
				if mode == 0 {
					return filterPass, j + 1, nil
				}
				// Mode 1 or 2: rewrite to pane-only erase.
				_, _, xStart, termW := sf.paneDims()
				paneW := termW - xStart + 1

				return filterRewrite, j + 1, buildPaneLineErase(xStart, paneW)
			}

			return filterPass, j + 1, nil
		}

		// Only digits and ; are valid parameter bytes.
		if (b < '0' || b > '9') && b != ';' {
			// Not a simple parameterized sequence — pass through.
			return filterPass, j + 1, nil
		}
	}

	return filterPass, 0, nil // incomplete
}

// containsByte reports whether b contains the byte c.
func containsByte(b []byte, c byte) bool {
	return bytes.IndexByte(b, c) >= 0
}

// --- Cursor coordinate translation helpers ---

// parseTwoParams parses "row;col" from parameter bytes.
// Missing or zero values default to 1 per ECMA-48.
func parseTwoParams(params []byte) (p1, p2 int) {
	semi := bytes.IndexByte(params, ';')
	if semi < 0 {
		p1 = parseParamBytes(params)
		p2 = 1
	} else {
		p1 = parseParamBytes(params[:semi])
		p2 = parseParamBytes(params[semi+1:])
	}

	if p1 == 0 {
		p1 = 1
	}

	if p2 == 0 {
		p2 = 1
	}

	return p1, p2
}

// parseOneParam parses a single numeric parameter. Default is 1 if empty or zero.
func parseOneParam(params []byte) int {
	v := parseParamBytes(params)
	if v == 0 {
		return 1
	}

	return v
}

// parseParamBytes converts ASCII digit bytes to an int. Returns 0 for empty input.
func parseParamBytes(b []byte) int {
	v := 0

	for _, c := range b {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		}
	}

	return v
}

// clampInt clamps v to [low, high].
func clampInt(v, low, high int) int {
	if v < low {
		return low
	}

	if v > high {
		return high
	}

	return v
}

// buildCUP builds a CSI row;col <final> sequence (e.g. CSI 5;38 H).
func buildCUP(row, col int, final byte) []byte {
	var buf [20]byte // ESC [ <digits> ; <digits> <final> — max ~14 bytes

	n := copy(buf[:], "\x1b[")
	n += appendIntBytes(buf[n:], row)
	buf[n] = ';'
	n++
	n += appendIntBytes(buf[n:], col)
	buf[n] = final
	n++

	out := make([]byte, n)
	copy(out, buf[:n])

	return out
}

// buildSingleParam builds a CSI n <final> sequence (e.g. CSI 38 G).
func buildSingleParam(n int, final byte) []byte {
	var buf [12]byte // ESC [ <digits> <final> — max ~9 bytes

	pos := copy(buf[:], "\x1b[")
	pos += appendIntBytes(buf[pos:], n)
	buf[pos] = final
	pos++

	out := make([]byte, pos)
	copy(out, buf[:pos])

	return out
}

// buildPaneLineErase builds a sequence that erases only the pane portion of the
// current line: DECSC + CHA(xStart) + ECH(paneWidth) + DECRC.
func buildPaneLineErase(xStart, paneWidth int) []byte {
	var buf [32]byte // ESC7 + CSI<digits>G + CSI<digits>X + ESC8 — max ~22 bytes

	n := copy(buf[:], "\x1b7\x1b[")
	n += appendIntBytes(buf[n:], xStart)
	buf[n] = 'G'
	n++
	n += copy(buf[n:], "\x1b[")
	n += appendIntBytes(buf[n:], paneWidth)
	buf[n] = 'X'
	n++
	n += copy(buf[n:], "\x1b8")

	out := make([]byte, n)
	copy(out, buf[:n])

	return out
}

// appendIntBytes writes the decimal representation of n into dst and returns
// the number of bytes written. dst must have sufficient capacity.
func appendIntBytes(dst []byte, n int) int {
	if n == 0 {
		dst[0] = '0'
		return 1
	}

	// Write digits in reverse, then swap.
	i := 0
	for n > 0 {
		dst[i] = byte('0' + n%10)
		n /= 10
		i++
	}

	// Reverse.
	for l, r := 0, i-1; l < r; l, r = l+1, r-1 {
		dst[l], dst[r] = dst[r], dst[l]
	}

	return i
}
