//go:build unix

package harness

import (
	"bytes"
	"sync"
	"testing"
)

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

func TestLockedWriterOnWriteAfterWrite(t *testing.T) {
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
