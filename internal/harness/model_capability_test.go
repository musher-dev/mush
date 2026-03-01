//go:build unix

package harness

import (
	"bytes"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
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

		// Probed terminals (let the runtime probe decide).
		{name: "alacritty", term: "alacritty", want: true},
		{name: "kitty", term: "kitty", want: true},
		{name: "foot", term: "foot", want: true},
		{name: "unknown", term: "unknown", want: true},

		// Rejected terminals (no probe attempted).
		{name: "dumb", term: "dumb", want: false},
		{name: "empty", term: "", want: false},

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
			ctrl := &TerminalController{}
			ctrl.lrMarginSupported.Store(tc.lrMarginSupport)
			ctrl.sidebarForcedOff.Store(tc.forcedOff)
			ctrl.sidebarUserOff.Store(tc.userOff)

			m := &RootModel{term: ctrl}

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

func TestStripTerminalResponses(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "empty input",
			input: []byte{},
			want:  nil,
		},
		{
			name:  "DECRQM only",
			input: []byte("\x1b[?69;1$y"),
			want:  nil,
		},
		{
			name:  "CPR only",
			input: []byte("\x1b[12;5R"),
			want:  nil,
		},
		{
			name:  "both DECRQM and CPR",
			input: []byte("\x1b[?69;2$y\x1b[1;10R"),
			want:  nil,
		},
		{
			name:  "user input only",
			input: []byte("hello"),
			want:  []byte("hello"),
		},
		{
			name:  "user input before DECRQM",
			input: []byte("ab\x1b[?69;1$y"),
			want:  []byte("ab"),
		},
		{
			name:  "user input after CPR",
			input: []byte("\x1b[12;5Rxy"),
			want:  []byte("xy"),
		},
		{
			name:  "interleaved user input and responses",
			input: []byte("a\x1b[?69;3$yb\x1b[1;10Rc"),
			want:  []byte("abc"),
		},
		{
			name:  "arrow key escape sequence preserved",
			input: []byte("\x1b[?69;1$y\x1b[A\x1b[24;5R"),
			want:  []byte("\x1b[A"),
		},
		{
			name:  "multiple arrow keys preserved",
			input: []byte("\x1b[A\x1b[B\x1b[C"),
			want:  []byte("\x1b[A\x1b[B\x1b[C"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripTerminalResponses(tc.input)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("stripTerminalResponses(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMatchDECRQM69(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantOK  bool
		wantEnd int
	}{
		{name: "mode 0", input: []byte("\x1b[?69;0$y"), wantOK: true, wantEnd: 9},
		{name: "mode 1", input: []byte("\x1b[?69;1$y"), wantOK: true, wantEnd: 9},
		{name: "mode 2", input: []byte("\x1b[?69;2$y"), wantOK: true, wantEnd: 9},
		{name: "mode 3", input: []byte("\x1b[?69;3$y"), wantOK: true, wantEnd: 9},
		{name: "mode 4", input: []byte("\x1b[?69;4$y"), wantOK: true, wantEnd: 9},
		{name: "invalid digit 5", input: []byte("\x1b[?69;5$y"), wantOK: false},
		{name: "invalid digit 9", input: []byte("\x1b[?69;9$y"), wantOK: false},
		{name: "truncated", input: []byte("\x1b[?69;1$"), wantOK: false},
		{name: "too short", input: []byte("\x1b[?69"), wantOK: false},
		{name: "wrong prefix", input: []byte("\x1b[?70;1$y"), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, end := matchDECRQM69(tc.input, 0)
			if ok != tc.wantOK {
				t.Fatalf("matchDECRQM69 ok = %v, want %v", ok, tc.wantOK)
			}

			if ok && end != tc.wantEnd {
				t.Fatalf("matchDECRQM69 end = %d, want %d", end, tc.wantEnd)
			}
		})
	}
}

func TestMatchCPR(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantOK  bool
		wantEnd int
	}{
		{name: "simple", input: []byte("\x1b[1;1R"), wantOK: true, wantEnd: 6},
		{name: "multi-digit", input: []byte("\x1b[24;80R"), wantOK: true, wantEnd: 8},
		{name: "large values", input: []byte("\x1b[999;999R"), wantOK: true, wantEnd: 10},
		{name: "missing semicolon", input: []byte("\x1b[123R"), wantOK: false},
		{name: "no trailing R", input: []byte("\x1b[1;1X"), wantOK: false},
		{name: "non-digit row", input: []byte("\x1b[a;1R"), wantOK: false},
		{name: "non-digit col", input: []byte("\x1b[1;aR"), wantOK: false},
		{name: "too short", input: []byte("\x1b["), wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, end := matchCPR(tc.input, 0)
			if ok != tc.wantOK {
				t.Fatalf("matchCPR ok = %v, want %v", ok, tc.wantOK)
			}

			if ok && end != tc.wantEnd {
				t.Fatalf("matchCPR end = %d, want %d", end, tc.wantEnd)
			}
		})
	}
}

func TestProbeLRMarginSupportReturnsLeftover(t *testing.T) {
	// Use raw syscall pipes to avoid Go's internal poller, which interferes
	// with the SetNonblock calls inside probeLRMarginSupport.
	var stdinFDs, stdoutFDs [2]int

	if err := syscall.Pipe(stdinFDs[:]); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Pipe(stdoutFDs[:]); err != nil {
		syscall.Close(stdinFDs[0])
		syscall.Close(stdinFDs[1])
		t.Fatal(err)
	}

	stdinR := os.NewFile(uintptr(stdinFDs[0]), "stdin-r")
	stdinW := os.NewFile(uintptr(stdinFDs[1]), "stdin-w")
	stdoutR := os.NewFile(uintptr(stdoutFDs[0]), "stdout-r")
	stdoutW := os.NewFile(uintptr(stdoutFDs[1]), "stdout-w")

	defer stdinR.Close()
	defer stdinW.Close()
	defer stdoutR.Close()
	defer stdoutW.Close()

	// Write DECRQM response + CPR response + user keystrokes into the pipe.
	// DECRQM: mode 69 set (supported). CPR: row=1, col=5 (margins active).
	// User typed "hi" during the probe window.
	termResponses := []byte("\x1b[?69;1$y\x1b[1;5Rhi")

	go func() {
		time.Sleep(10 * time.Millisecond)

		_, _ = stdinW.Write(termResponses)
	}()

	// Drain stdout in the background to prevent blocking.
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdoutR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// termWidth=20, probeLeft = max(2, 20/4) = 5, probeRight = max(7, 10) = 10.
	// CPR col=5 == probeLeft → result should be true.
	supported, userInput := probeLRMarginSupport(stdinR, stdoutW, 200*time.Millisecond, 20)

	if !supported {
		t.Fatal("expected supported=true")
	}

	if !bytes.Equal(userInput, []byte("hi")) {
		t.Fatalf("userInput = %q, want %q", userInput, "hi")
	}
}
