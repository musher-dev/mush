//go:build unix

package harness

import "strings"

const maxTermSeqTailBytes = 16

type terminalEvent uint8

const (
	terminalEventReset terminalEvent = iota
	terminalEventSoftReset
	terminalEventScrollReset
	terminalEventDisableLR
	terminalEventAltEnter
	terminalEventAltExit
	terminalEventEraseDisplay
)

func (tc *TerminalController) inspectTerminalControlSequences(p []byte) {
	if len(p) == 0 {
		return
	}

	tc.termSeqMu.Lock()
	events, tail := parseTerminalEvents(tc.termSeqTail, p)
	tc.termSeqTail = tail
	tc.termSeqMu.Unlock()

	for _, ev := range events {
		tc.handleTerminalEvent(ev)
	}
}

func (tc *TerminalController) handleTerminalEvent(ev terminalEvent) {
	switch ev {
	case terminalEventAltEnter:
		tc.altScreenActive.Store(true)
	case terminalEventAltExit:
		wasActive := tc.altScreenActive.Swap(false)
		if wasActive {
			tc.restoreLayoutAfterAltScreen()
		}
	case terminalEventReset, terminalEventSoftReset:
		// Hard/soft reset nukes all terminal state — permanent disable.
		tc.disableSidebar()
	case terminalEventScrollReset, terminalEventDisableLR:
		// Common harness init sequences — reassert layout or quarantine.
		tc.reassertOrQuarantine(ev)
	case terminalEventEraseDisplay:
		if tc.SidebarEnabled() {
			tc.scheduleSidebarRedraw()
		}
	}
}

func parseTerminalEvents(tail, chunk []byte) (events []terminalEvent, newTail []byte) {
	combined := make([]byte, 0, len(tail)+len(chunk))
	combined = append(combined, tail...)
	combined = append(combined, chunk...)

	i := 0

	for i < len(combined) {
		if combined[i] != 0x1b {
			i++
			continue
		}

		if i+1 >= len(combined) {
			break
		}

		next := combined[i+1]
		if next == 'c' {
			events = append(events, terminalEventReset)
			i += 2

			continue
		}

		if next != '[' {
			i += 2
			continue
		}

		j := i + 2
		for ; j < len(combined); j++ {
			if combined[j] >= 0x40 && combined[j] <= 0x7e {
				break
			}
		}

		if j >= len(combined) {
			break
		}

		params := string(combined[i+2 : j])
		final := combined[j]

		if ev, ok := terminalEventFromCSI(params, final); ok {
			events = append(events, ev)
		}

		i = j + 1
	}

	rem := combined[i:]
	if len(rem) > maxTermSeqTailBytes {
		rem = rem[len(rem)-maxTermSeqTailBytes:]
	}

	newTail = make([]byte, len(rem))
	copy(newTail, rem)

	return events, newTail
}

func terminalEventFromCSI(params string, final byte) (terminalEvent, bool) {
	switch {
	case final == 'p' && params == "!":
		return terminalEventSoftReset, true
	case final == 'r' && params == "":
		return terminalEventScrollReset, true
	case final == 'l' && params == "?69":
		return terminalEventDisableLR, true
	case final == 's' && isDECSLRMParams(params):
		// Parameterized CSI <digits>;<digits> s is DECSLRM — the child is
		// taking over left/right margin control. Disable the sidebar.
		return terminalEventDisableLR, true
	case final == 'J' && (params == "" || params == "0" || params == "1" || params == "2"):
		return terminalEventEraseDisplay, true
	case (final == 'h' || final == 'l') && strings.HasPrefix(params, "?"):
		mode := strings.TrimPrefix(params, "?")
		switch mode {
		case "47", "1047", "1049":
			if final == 'h' {
				return terminalEventAltEnter, true
			}

			return terminalEventAltExit, true
		}
	}

	return 0, false
}

// isDECSLRMParams reports whether params matches the DECSLRM grammar:
// one or two groups of digits separated by a semicolon (e.g. "1;40", "5;",
// ";80", "10"). No private-mode prefix (?) or other non-digit characters.
func isDECSLRMParams(params string) bool {
	if params == "" {
		return false
	}

	semi := false

	for i := 0; i < len(params); i++ {
		c := params[i]

		switch {
		case c >= '0' && c <= '9':
			// digit — ok
		case c == ';' && !semi:
			semi = true
		default:
			return false
		}
	}

	return true
}

// isDECSLRMBytes is the allocation-free byte-slice variant of isDECSLRMParams
// for use on the PTY output hot path.
func isDECSLRMBytes(params []byte) bool {
	if len(params) == 0 {
		return false
	}

	semi := false

	for _, c := range params {
		switch {
		case c >= '0' && c <= '9':
			// digit — ok
		case c == ';' && !semi:
			semi = true
		default:
			return false
		}
	}

	return true
}
