//go:build unix

package harness

import (
	"bytes"
	"sync"
	"testing"
)

func TestSupportsLRMargins(t *testing.T) {
	tests := []struct {
		name string
		term string
		want bool
	}{
		// Allowed terminals.
		{name: "xterm-256color", term: "xterm-256color", want: true},
		{name: "xterm", term: "xterm", want: true},
		{name: "tmux-256color", term: "tmux-256color", want: true},
		{name: "tmux", term: "tmux", want: true},
		{name: "screen", term: "screen", want: true},
		{name: "screen-256color", term: "screen-256color", want: true},
		{name: "wezterm", term: "wezterm", want: true},
		{name: "ghostty", term: "ghostty", want: true},

		// Rejected terminals (no DECLRMM/DECSLRM support).
		{name: "alacritty", term: "alacritty", want: false},
		{name: "kitty", term: "kitty", want: false},
		{name: "kitty-direct", term: "xterm-kitty", want: true}, // contains "xterm"
		{name: "dumb", term: "dumb", want: false},
		{name: "empty", term: "", want: false},
		{name: "foot", term: "foot", want: false},
		{name: "unknown", term: "unknown", want: false},

		// Edge cases.
		{name: "case insensitive upper", term: "XTERM-256COLOR", want: true},
		{name: "case insensitive mixed", term: "Ghostty", want: true},
		{name: "whitespace trimmed", term: "  xterm  ", want: true},
		{name: "whitespace only", term: "   ", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := supportsLRMargins(tc.term)
			if got != tc.want {
				t.Fatalf("supportsLRMargins(%q) = %v, want %v", tc.term, got, tc.want)
			}
		})
	}
}

func TestParseDECRQM69Response(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		wantSupported bool
		wantDecided   bool
	}{
		{name: "set state", input: []byte("\x1b[?69;1$y"), wantSupported: true, wantDecided: true},
		{name: "reset state", input: []byte("\x1b[?69;2$y"), wantSupported: true, wantDecided: true},
		{name: "permanent state", input: []byte("\x1b[?69;3$y"), wantSupported: true, wantDecided: true},
		{name: "not recognized", input: []byte("\x1b[?69;0$y"), wantSupported: false, wantDecided: true},
		{name: "permanently reset", input: []byte("\x1b[?69;4$y"), wantSupported: false, wantDecided: true},
		{name: "no response", input: []byte("noise"), wantSupported: false, wantDecided: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSupported, gotDecided := parseDECRQM69Response(tc.input)
			if gotSupported != tc.wantSupported || gotDecided != tc.wantDecided {
				t.Fatalf("parseDECRQM69Response(%q) = (%v,%v), want (%v,%v)",
					string(tc.input), gotSupported, gotDecided, tc.wantSupported, tc.wantDecided)
			}
		})
	}
}

func TestParseCPRColumn(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantCol int
		wantOK  bool
	}{
		{name: "single cpr", input: []byte("\x1b[12;5R"), wantCol: 5, wantOK: true},
		{name: "mixed stream picks latest", input: []byte("x\x1b[1;2Rz\x1b[9;15R"), wantCol: 15, wantOK: true},
		{name: "invalid payload", input: []byte("\x1b[fooR"), wantOK: false},
		{name: "no cpr", input: []byte("hello"), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col, ok := parseCPRColumn(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseCPRColumn ok = %v, want %v", ok, tc.wantOK)
			}

			if ok && col != tc.wantCol {
				t.Fatalf("parseCPRColumn col = %d, want %d", col, tc.wantCol)
			}
		})
	}
}

func TestLockedWriterCallsOnWriteAfterUnderlyingWrite(t *testing.T) {
	var buf bytes.Buffer

	var mu sync.Mutex

	var callOrder []string

	lw := &lockedWriter{
		mu: &mu,
		w: writerFunc(func(p []byte) (int, error) {
			callOrder = append(callOrder, "write")
			return buf.Write(p)
		}),
		onWrite: func(p []byte) {
			callOrder = append(callOrder, "onWrite")
		},
	}

	data := []byte("hello")

	n, err := lw.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if n != len(data) {
		t.Fatalf("Write n = %d, want %d", n, len(data))
	}

	if buf.String() != "hello" {
		t.Fatalf("buffer = %q, want %q", buf.String(), "hello")
	}

	if len(callOrder) != 2 || callOrder[0] != "write" || callOrder[1] != "onWrite" {
		t.Fatalf("call order = %v, want [write onWrite]", callOrder)
	}
}

// writerFunc adapts a function into an io.Writer for testing.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestSidebarEnabledCombinations(t *testing.T) {
	tests := []struct {
		name            string
		lrMarginSupport bool
		forcedOff       bool
		userOff         bool
		want            bool
	}{
		{"all enabled", true, false, false, true},
		{"no LR margin support", false, false, false, false},
		{"forced off", true, true, false, false},
		{"user off", true, false, true, false},
		{"forced and user off", true, true, true, false},
		{"no LR and user off", false, false, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &RootModel{
				lrMarginSupported: tc.lrMarginSupport,
			}
			m.sidebarForcedOff.Store(tc.forcedOff)
			m.sidebarUserOff.Store(tc.userOff)

			got := m.sidebarEnabled()
			if got != tc.want {
				t.Fatalf("sidebarEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseTerminalEvents(t *testing.T) {
	tests := []struct {
		name      string
		tail      []byte
		chunk     []byte
		wantEvent []terminalEvent
	}{
		{
			name:      "disable lr margin mode",
			chunk:     []byte("\x1b[?69l"),
			wantEvent: []terminalEvent{terminalEventDisableLR},
		},
		{
			name:      "scroll reset",
			chunk:     []byte("\x1b[r"),
			wantEvent: []terminalEvent{terminalEventScrollReset},
		},
		{
			name:      "soft reset",
			chunk:     []byte("\x1b[!p"),
			wantEvent: []terminalEvent{terminalEventSoftReset},
		},
		{
			name:      "alt enter",
			chunk:     []byte("\x1b[?1049h"),
			wantEvent: []terminalEvent{terminalEventAltEnter},
		},
		{
			name:      "alt exit",
			chunk:     []byte("\x1b[?1049l"),
			wantEvent: []terminalEvent{terminalEventAltExit},
		},
		{
			name:      "hard reset",
			chunk:     []byte("\x1bc"),
			wantEvent: []terminalEvent{terminalEventReset},
		},
		{
			name:      "sequence split across chunks",
			tail:      []byte("\x1b[?1049"),
			chunk:     []byte("h"),
			wantEvent: []terminalEvent{terminalEventAltEnter},
		},
		{
			name:      "scroll region set is not scroll reset",
			chunk:     []byte("\x1b[2;24r"),
			wantEvent: nil,
		},
		{
			name:      "scroll region set full screen is not scroll reset",
			chunk:     []byte("\x1b[1;50r"),
			wantEvent: nil,
		},
		{
			name:      "regular output",
			chunk:     []byte("hello world"),
			wantEvent: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, tail := parseTerminalEvents(tc.tail, tc.chunk)
			if len(events) != len(tc.wantEvent) {
				t.Fatalf("events len = %d, want %d (%v)", len(events), len(tc.wantEvent), events)
			}

			for i := range events {
				if events[i] != tc.wantEvent[i] {
					t.Fatalf("events[%d] = %v, want %v", i, events[i], tc.wantEvent[i])
				}
			}

			if len(tail) > maxTermSeqTailBytes {
				t.Fatalf("tail length = %d, want <= %d", len(tail), maxTermSeqTailBytes)
			}
		})
	}
}
