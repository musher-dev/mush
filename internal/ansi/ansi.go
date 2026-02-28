package ansi

import "strings"

// ANSI parser states for ECMA-48 compliant escape sequence stripping.
type parserState int

const (
	stNormal          parserState = iota
	stEscSeen                     // ESC received, waiting for dispatch byte
	stEscIntermediate             // ESC + intermediate byte (0x20-0x2F) — nF escape
	stCSI                         // Inside CSI sequence (ESC [)
	stOSC                         // Inside OSC sequence (ESC ])
	stOSCEsc                      // ESC seen inside OSC (possible ST = ESC \)
	stStringSeq                   // Inside DCS/PM/APC/SOS string (ESC P, ESC ^, ESC _, ESC X)
	stStringEsc                   // ESC seen inside string sequence (possible ST)
)

// Strip removes ANSI escape sequences from a string.
//
// Handles CSI (ESC [), OSC (ESC ]), DCS (ESC P), PM (ESC ^), APC (ESC _),
// SOS (ESC X), nF escapes (ESC + 0x20-0x2F intermediate bytes + final byte),
// and Fe two-byte escapes (ESC followed by 0x40-0x7E). CSI final bytes are
// the full ECMA-48 range 0x40-0x7E (not just letters), so sequences ending
// in ~, @, etc. are correctly consumed.
//
// If the string ends mid-escape, the buffered escape bytes are emitted
// verbatim (not silently discarded).
func Strip(s string) string {
	var b strings.Builder

	state := stNormal

	var escBuf []rune

	for _, r := range s {
		switch state {
		case stNormal:
			if r == '\x1b' {
				state = stEscSeen
				escBuf = escBuf[:0]
				escBuf = append(escBuf, r)

				continue
			}

			b.WriteRune(r)

		case stEscSeen:
			escBuf = append(escBuf, r)

			switch {
			case r == '[':
				state = stCSI
			case r == ']':
				state = stOSC
			case r == 'P', r == 'X', r == '^', r == '_':
				state = stStringSeq
			case r >= 0x20 && r <= 0x2F:
				// nF escape: intermediate byte(s) followed by final byte
				state = stEscIntermediate
			case r >= 0x30 && r <= 0x7E:
				// Fp (0x30-0x3F, DEC private like ESC7/ESC8) or
				// Fe (0x40-0x5F) or Fs (0x60-0x7E) — two-byte escape, consume both
				state = stNormal
				escBuf = escBuf[:0]
			default:
				// Not a recognized escape introducer — emit buffered bytes
				for _, br := range escBuf {
					b.WriteRune(br)
				}

				escBuf = escBuf[:0]
				state = stNormal
			}

		case stEscIntermediate:
			escBuf = append(escBuf, r)

			switch {
			case r >= 0x20 && r <= 0x2F:
				// More intermediate bytes — stay in this state
			case r >= 0x30 && r <= 0x7E:
				// Final byte — consume the entire nF escape
				state = stNormal
				escBuf = escBuf[:0]
			default:
				// Invalid nF sequence — emit buffered bytes verbatim
				for _, br := range escBuf {
					b.WriteRune(br)
				}

				escBuf = escBuf[:0]
				state = stNormal
			}

		case stCSI:
			escBuf = append(escBuf, r)

			// CSI final byte: 0x40-0x7E per ECMA-48
			if r >= 0x40 && r <= 0x7E {
				state = stNormal
				escBuf = escBuf[:0]
			}

		case stOSC:
			escBuf = append(escBuf, r)

			switch r {
			case '\x07':
				// BEL terminates OSC
				state = stNormal
				escBuf = escBuf[:0]
			case '\x1b':
				state = stOSCEsc
			}

		case stOSCEsc:
			escBuf = append(escBuf, r)

			if r == '\\' {
				// ST (ESC \) terminates OSC
				state = stNormal
				escBuf = escBuf[:0]
			} else {
				// Not ST — back to OSC
				state = stOSC
			}

		case stStringSeq:
			escBuf = append(escBuf, r)

			if r == '\x1b' {
				state = stStringEsc
			}

		case stStringEsc:
			escBuf = append(escBuf, r)

			if r == '\\' {
				// ST (ESC \) terminates string sequence
				state = stNormal
				escBuf = escBuf[:0]
			} else {
				// Not ST — back to string sequence
				state = stStringSeq
			}
		}
	}

	// If we ended mid-escape, emit the buffered bytes verbatim
	if state != stNormal {
		for _, r := range escBuf {
			b.WriteRune(r)
		}
	}

	return b.String()
}
