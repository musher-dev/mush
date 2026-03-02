//go:build unix

package harness

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"
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
		altScreen       bool
		want            bool
	}{
		{"all enabled", true, false, false, false, true},
		{"no LR margin support", false, false, false, false, false},
		{"forced off", true, true, false, false, false},
		{"user off", true, false, true, false, false},
		{"forced and user off", true, true, true, false, false},
		{"no LR and user off", false, false, true, false, false},
		{"alt screen active", true, false, false, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &TerminalController{}
			ctrl.lrMarginSupported.Store(tc.lrMarginSupport)
			ctrl.sidebarForcedOff.Store(tc.forcedOff)
			ctrl.sidebarUserOff.Store(tc.userOff)
			ctrl.altScreenActive.Store(tc.altScreen)

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

func newTestFilter(active bool) *sidebarFilter {
	// 140-col terminal, sidebar occupies cols 1-37, pane starts at col 38.
	return newTestFilterWithDims(active, 2, 24, 38, 140)
}

func newTestFilterWithDims(active bool, contentTop, scrollBottom, paneXStart, termWidth int) *sidebarFilter {
	return &sidebarFilter{
		active: func() bool { return active },
		paneDims: func() (int, int, int, int) {
			return contentTop, scrollBottom, paneXStart, termWidth
		},
	}
}

func TestSidebarFilter(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		input  []byte
		want   []byte
	}{
		{
			name:   "bare CSI s rewritten when active",
			active: true,
			input:  []byte("\x1b[s"),
			want:   []byte("\x1b7"),
		},
		{
			name:   "bare CSI u rewritten when active",
			active: true,
			input:  []byte("\x1b[u"),
			want:   []byte("\x1b8"),
		},
		{
			name:   "bare CSI s passthrough when inactive",
			active: false,
			input:  []byte("\x1b[s"),
			want:   []byte("\x1b[s"),
		},
		{
			name:   "bare CSI u passthrough when inactive",
			active: false,
			input:  []byte("\x1b[u"),
			want:   []byte("\x1b[u"),
		},
		{
			name:   "plain text passthrough",
			active: true,
			input:  []byte("hello world"),
			want:   []byte("hello world"),
		},
		{
			name:   "mixed text and bare CSI s",
			active: true,
			input:  []byte("abc\x1b[sdef\x1b[ughij"),
			want:   []byte("abc\x1b7def\x1b8ghij"),
		},
		{
			name:   "multiple sequences in one chunk",
			active: true,
			input:  []byte("\x1b[s\x1b[u\x1b[s"),
			want:   []byte("\x1b7\x1b8\x1b7"),
		},
		{
			name:   "CUP translated, erase display passes through",
			active: true,
			input:  []byte("\x1b[H\x1b[2J"),
			want:   []byte("\x1b[2;38H\x1b[2J"),
		},
		{
			name:   "ESC c (hard reset) passes through",
			active: true,
			input:  []byte("\x1bc"),
			want:   []byte("\x1bc"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(tc.active)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.want) {
				t.Fatalf("rewrite(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSidebarFilterNoAllocForSGR(t *testing.T) {
	// Chunks with ESC sequences that are NOT bare CSI s/u should return
	// the original slice without allocation (lazy-alloc optimization).
	sf := newTestFilter(true)

	sgr := []byte("\x1b[31mhello\x1b[0m") // red text + reset
	got := sf.rewrite(sgr)

	// Should be the exact same slice (pointer equality).
	if &got[0] != &sgr[0] {
		t.Fatal("expected zero-copy passthrough for SGR-only chunk")
	}
}

func TestSidebarFilterChunkBoundary(t *testing.T) {
	t.Run("ESC at end of chunk 1 then [s in chunk 2", func(t *testing.T) {
		sf := newTestFilter(true)

		got1 := sf.rewrite([]byte("abc\x1b"))
		got1 = append(got1, sf.rewrite([]byte("[sdef"))...)
		want := []byte("abc\x1b7def")

		if !bytes.Equal(got1, want) {
			t.Fatalf("got %q, want %q", got1, want)
		}
	})

	t.Run("ESC [ at end of chunk 1 then s in chunk 2", func(t *testing.T) {
		sf := newTestFilter(true)

		got1 := sf.rewrite([]byte("abc\x1b["))
		got1 = append(got1, sf.rewrite([]byte("sdef"))...)
		want := []byte("abc\x1b7def")

		if !bytes.Equal(got1, want) {
			t.Fatalf("got %q, want %q", got1, want)
		}
	})

	t.Run("ESC [ at end of chunk 1 then parameterized s in chunk 2 drops DECSLRM", func(t *testing.T) {
		sf := newTestFilter(true)

		got1 := sf.rewrite([]byte("abc\x1b["))
		got1 = append(got1, sf.rewrite([]byte("1;40s"))...)
		// DECSLRM (parameterized CSI s) should be dropped when active.
		want := []byte("abc")

		if !bytes.Equal(got1, want) {
			t.Fatalf("got %q, want %q", got1, want)
		}
	})

	t.Run("held tail flushed when active returns false", func(t *testing.T) {
		active := true
		sf := &sidebarFilter{
			active:   func() bool { return active },
			paneDims: func() (int, int, int, int) { return 2, 24, 38, 140 },
		}

		// First chunk: active, ESC at end → held in tail.
		got1 := sf.rewrite([]byte("abc\x1b"))

		// Deactivate before next chunk.
		active = false

		got1 = append(got1, sf.rewrite([]byte("[sdef"))...)
		// When inactive, the held ESC is flushed verbatim along with new data.
		want := []byte("abc\x1b[sdef")

		if !bytes.Equal(got1, want) {
			t.Fatalf("got %q, want %q", got1, want)
		}
	})

	t.Run("ESC [ at end of chunk 1 then u in chunk 2", func(t *testing.T) {
		sf := newTestFilter(true)

		got1 := sf.rewrite([]byte("\x1b["))
		got1 = append(got1, sf.rewrite([]byte("u"))...)
		want := []byte("\x1b8")

		if !bytes.Equal(got1, want) {
			t.Fatalf("got %q, want %q", got1, want)
		}
	})
}

func TestSidebarFilterBareCSIRRewrite(t *testing.T) {
	sf := newTestFilter(true) // paneDims returns (2, 24, 38, 140)
	got := sf.rewrite([]byte("\x1b[r"))
	want := []byte("\x1b[2;24r")

	if !bytes.Equal(got, want) {
		t.Fatalf("rewrite(bare CSI r) = %q, want %q", got, want)
	}
}

func TestSidebarFilterParameterizedCSIRPassthrough(t *testing.T) {
	sf := newTestFilter(true)
	input := []byte("\x1b[2;24r")
	got := sf.rewrite(input)

	if !bytes.Equal(got, input) {
		t.Fatalf("rewrite(parameterized CSI r) = %q, want %q", got, input)
	}
}

func TestSidebarFilterCSI69lDropped(t *testing.T) {
	sf := newTestFilter(true)
	got := sf.rewrite([]byte("\x1b[?69l"))

	if len(got) != 0 {
		t.Fatalf("rewrite(CSI ?69l) = %q, want empty", got)
	}
}

func TestSidebarFilterCSI69lPassthroughInactive(t *testing.T) {
	sf := newTestFilter(false)
	input := []byte("\x1b[?69l")
	got := sf.rewrite(input)

	if !bytes.Equal(got, input) {
		t.Fatalf("rewrite(CSI ?69l inactive) = %q, want %q", got, input)
	}
}

func TestSidebarFilterCSI25lPassthrough(t *testing.T) {
	sf := newTestFilter(true)
	input := []byte("\x1b[?25l") // hide cursor — should not be dropped
	got := sf.rewrite(input)

	if !bytes.Equal(got, input) {
		t.Fatalf("rewrite(CSI ?25l) = %q, want %q", got, input)
	}
}

func TestSidebarFilterDECSLRMDropped(t *testing.T) {
	sf := newTestFilter(true)
	got := sf.rewrite([]byte("\x1b[1;40s"))

	if len(got) != 0 {
		t.Fatalf("rewrite(DECSLRM) = %q, want empty", got)
	}
}

func TestSidebarFilterCSI69lChunkBoundary(t *testing.T) {
	sf := newTestFilter(true)

	// Split CSI ?69l across two chunks: ESC[?69 | l
	got := sf.rewrite([]byte("\x1b[?69"))
	got = append(got, sf.rewrite([]byte("l"))...)

	if len(got) != 0 {
		t.Fatalf("rewrite(CSI ?69l split) = %q, want empty", got)
	}
}

func TestSidebarFilterTailOverflow(t *testing.T) {
	sf := newTestFilter(true)
	// A very long CSI sequence (>16 bytes) should flush through on overflow.
	// ESC [ ? 1 2 3 4 5 6 7 8 9 0 1 2 3 l  = 18 bytes, exceeds 16-byte tail.
	input := []byte("\x1b[?1234567890123l")
	got := sf.rewrite(input)

	// Should pass through (not matched as ?69l, too long to hold in tail).
	if !bytes.Equal(got, input) {
		t.Fatalf("rewrite(overflow) = %q, want %q", got, input)
	}
}

// --- Cursor coordinate translation tests ---
// Test filter uses paneDims returning (contentTop=2, scrollBottom=24, paneXStart=38, termWidth=140).
// Child coordinates are 1-based relative to the pane.
// Translation: row → clamp(row + 2 - 1, 2, 24), col → clamp(col + 38 - 1, 38, 140).

func TestSidebarFilterCUPTranslation(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "bare CUP (CSI H) → home to pane origin",
			input: []byte("\x1b[H"),
			want:  []byte("\x1b[2;38H"),
		},
		{
			name:  "bare CUP (CSI f) → home to pane origin",
			input: []byte("\x1b[f"),
			want:  []byte("\x1b[2;38f"),
		},
		{
			name:  "CUP row=1 col=1 → pane origin",
			input: []byte("\x1b[1;1H"),
			want:  []byte("\x1b[2;38H"),
		},
		{
			name:  "CUP row=5 col=10 → translated",
			input: []byte("\x1b[5;10H"),
			want:  []byte("\x1b[6;47H"),
		},
		{
			name:  "CUP with zero params defaults to 1",
			input: []byte("\x1b[0;0H"),
			want:  []byte("\x1b[2;38H"),
		},
		{
			name:  "CUP semicolon only → defaults to 1;1",
			input: []byte("\x1b[;H"),
			want:  []byte("\x1b[2;38H"),
		},
		{
			name:  "CUP row only → col defaults to 1",
			input: []byte("\x1b[5H"),
			want:  []byte("\x1b[6;38H"),
		},
		{
			name:  "CUP row exceeds bounds → clamped to scrollBottom",
			input: []byte("\x1b[999;1H"),
			want:  []byte("\x1b[24;38H"),
		},
		{
			name:  "CUP col exceeds bounds → clamped to termWidth",
			input: []byte("\x1b[1;999H"),
			want:  []byte("\x1b[2;140H"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(true)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.want) {
				t.Fatalf("rewrite(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSidebarFilterCHATranslation(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "bare CHA (CSI G) → pane col start",
			input: []byte("\x1b[G"),
			want:  []byte("\x1b[38G"),
		},
		{
			name:  "CHA col=1 → pane col start",
			input: []byte("\x1b[1G"),
			want:  []byte("\x1b[38G"),
		},
		{
			name:  "CHA col=10 → translated",
			input: []byte("\x1b[10G"),
			want:  []byte("\x1b[47G"),
		},
		{
			name:  "CHA col exceeds bounds → clamped",
			input: []byte("\x1b[999G"),
			want:  []byte("\x1b[140G"),
		},
		{
			name:  "bare HPA (CSI `) → pane col start",
			input: []byte("\x1b[`"),
			want:  []byte("\x1b[38`"),
		},
		{
			name:  "HPA col=5 → translated",
			input: []byte("\x1b[5`"),
			want:  []byte("\x1b[42`"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(true)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.want) {
				t.Fatalf("rewrite(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSidebarFilterVPATranslation(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "bare VPA (CSI d) → pane top row",
			input: []byte("\x1b[d"),
			want:  []byte("\x1b[2d"),
		},
		{
			name:  "VPA row=1 → pane top row",
			input: []byte("\x1b[1d"),
			want:  []byte("\x1b[2d"),
		},
		{
			name:  "VPA row=5 → translated",
			input: []byte("\x1b[5d"),
			want:  []byte("\x1b[6d"),
		},
		{
			name:  "VPA row exceeds bounds → clamped",
			input: []byte("\x1b[999d"),
			want:  []byte("\x1b[24d"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(true)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.want) {
				t.Fatalf("rewrite(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSidebarFilterCursorPassthroughWhenInactive(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"CUP inactive", []byte("\x1b[5;10H")},
		{"CHA inactive", []byte("\x1b[10G")},
		{"HPA inactive", []byte("\x1b[10`")},
		{"VPA inactive", []byte("\x1b[5d")},
		{"bare CUP inactive", []byte("\x1b[H")},
		{"bare CHA inactive", []byte("\x1b[G")},
		{"bare VPA inactive", []byte("\x1b[d")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(false)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.input) {
				t.Fatalf("rewrite(%q) = %q, want passthrough %q", tc.input, got, tc.input)
			}
		})
	}
}

func TestSidebarFilterMixedCursorAndSGR(t *testing.T) {
	sf := newTestFilter(true) // paneDims: (2, 24, 38, 140)

	// SGR (color) + CUP + text + CHA + text
	input := []byte("\x1b[31m\x1b[1;1Hhello\x1b[1Gworld")
	got := sf.rewrite(input)
	want := []byte("\x1b[31m\x1b[2;38Hhello\x1b[38Gworld")

	if !bytes.Equal(got, want) {
		t.Fatalf("rewrite mixed = %q, want %q", got, want)
	}
}

func TestSidebarFilterCursorChunkBoundary(t *testing.T) {
	t.Run("CUP split across chunks", func(t *testing.T) {
		sf := newTestFilter(true)

		// Split CSI 5;10H: ESC[5;1 | 0H
		got := sf.rewrite([]byte("\x1b[5;1"))
		got = append(got, sf.rewrite([]byte("0H"))...)
		want := []byte("\x1b[6;47H")

		if !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("CHA split across chunks", func(t *testing.T) {
		sf := newTestFilter(true)

		// Split CSI 10G: ESC[ | 10G
		got := sf.rewrite([]byte("\x1b["))
		got = append(got, sf.rewrite([]byte("10G"))...)
		want := []byte("\x1b[47G")

		if !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("VPA split ESC at chunk end", func(t *testing.T) {
		sf := newTestFilter(true)

		got := sf.rewrite([]byte("text\x1b"))
		got = append(got, sf.rewrite([]byte("[5d"))...)
		want := []byte("text\x1b[6d")

		if !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestSidebarFilterDECSLRMStillDropped(t *testing.T) {
	sf := newTestFilter(true)

	// Parameterized CSI s (DECSLRM) should still be dropped.
	got := sf.rewrite([]byte("before\x1b[1;40safter"))

	want := []byte("beforeafter")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSidebarFilterExistingBehaviorUnchanged(t *testing.T) {
	sf := newTestFilter(true)

	// CSI s → DECSC, CSI u → DECRC (existing rewrites should still work)
	got := sf.rewrite([]byte("\x1b[s\x1b[u"))
	want := []byte("\x1b7\x1b8")

	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSidebarFilterCustomDims(t *testing.T) {
	// Narrower pane: contentTop=3, scrollBottom=30, paneXStart=50, termWidth=100
	sf := newTestFilterWithDims(true, 3, 30, 50, 100)

	// CUP row=1 col=1 → (3, 50)
	got := sf.rewrite([]byte("\x1b[1;1H"))
	want := []byte("\x1b[3;50H")

	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}

	// CUP row=28 col=51 → (30, 100) — both clamped
	got = sf.rewrite([]byte("\x1b[100;100H"))
	want = []byte("\x1b[30;100H")

	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// --- Helper function unit tests ---

func TestParseTwoParams(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		wantP1 int
		wantP2 int
	}{
		{"normal", []byte("5;10"), 5, 10},
		{"single param", []byte("5"), 5, 1},
		{"zero defaults to 1", []byte("0;0"), 1, 1},
		{"empty defaults to 1", []byte(""), 1, 1},
		{"semicolon only", []byte(";"), 1, 1},
		{"leading semicolon", []byte(";10"), 1, 10},
		{"trailing semicolon", []byte("5;"), 5, 1},
		{"large values", []byte("999;999"), 999, 999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p1, p2 := parseTwoParams(tc.input)
			if p1 != tc.wantP1 || p2 != tc.wantP2 {
				t.Fatalf("parseTwoParams(%q) = (%d, %d), want (%d, %d)",
					tc.input, p1, p2, tc.wantP1, tc.wantP2)
			}
		})
	}
}

func TestParseOneParam(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  int
	}{
		{"normal", []byte("5"), 5},
		{"zero defaults to 1", []byte("0"), 1},
		{"empty defaults to 1", []byte(""), 1},
		{"large value", []byte("999"), 999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOneParam(tc.input)
			if got != tc.want {
				t.Fatalf("parseOneParam(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{15, 1, 10, 10},
		{1, 1, 1, 1},
	}

	for _, tc := range tests {
		got := clampInt(tc.v, tc.lo, tc.hi)
		if got != tc.want {
			t.Fatalf("clampInt(%d, %d, %d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

func TestBuildCUP(t *testing.T) {
	got := buildCUP(5, 38, 'H')
	want := []byte("\x1b[5;38H")

	if !bytes.Equal(got, want) {
		t.Fatalf("buildCUP(5, 38, 'H') = %q, want %q", got, want)
	}
}

func TestBuildSingleParam(t *testing.T) {
	got := buildSingleParam(38, 'G')
	want := []byte("\x1b[38G")

	if !bytes.Equal(got, want) {
		t.Fatalf("buildSingleParam(38, 'G') = %q, want %q", got, want)
	}
}

func TestTerminalEventFromCSI_DECSLRM(t *testing.T) {
	t.Run("parameterized s triggers DisableLR", func(t *testing.T) {
		ev, ok := terminalEventFromCSI("1;40", 's')
		if !ok {
			t.Fatal("expected ok=true")
		}

		if ev != terminalEventDisableLR {
			t.Fatalf("event = %v, want terminalEventDisableLR", ev)
		}
	})

	t.Run("bare s produces no event", func(t *testing.T) {
		_, ok := terminalEventFromCSI("", 's')
		if ok {
			t.Fatal("expected ok=false for bare CSI s")
		}
	})

	t.Run("single number is valid DECSLRM", func(t *testing.T) {
		ev, ok := terminalEventFromCSI("10", 's')
		if !ok {
			t.Fatal("expected ok=true")
		}

		if ev != terminalEventDisableLR {
			t.Fatalf("event = %v, want terminalEventDisableLR", ev)
		}
	})

	t.Run("private mode prefix rejected", func(t *testing.T) {
		_, ok := terminalEventFromCSI("?1", 's')
		if ok {
			t.Fatal("expected ok=false for CSI ?1 s (XTSAVE)")
		}
	})

	t.Run("multiple semicolons rejected", func(t *testing.T) {
		_, ok := terminalEventFromCSI("1;2;3", 's')
		if ok {
			t.Fatal("expected ok=false for multiple semicolons")
		}
	})

	t.Run("non-digit characters rejected", func(t *testing.T) {
		_, ok := terminalEventFromCSI("1a;40", 's')
		if ok {
			t.Fatal("expected ok=false for non-digit characters")
		}
	})
}

func TestLockedWriterFilter(t *testing.T) {
	var buf bytes.Buffer

	var mu sync.Mutex

	var writtenToUnderlying []byte

	var onWriteReceived []byte

	lw := &lockedWriter{
		mu: &mu,
		w: writerFunc(func(p []byte) (int, error) {
			writtenToUnderlying = make([]byte, len(p))
			copy(writtenToUnderlying, p)

			return buf.Write(p)
		}),
		filter: func(p []byte) []byte {
			// Simulate rewriting: replace "ab" with "XY"
			return bytes.ReplaceAll(p, []byte("ab"), []byte("XY"))
		},
		onWrite: func(p []byte) {
			onWriteReceived = make([]byte, len(p))
			copy(onWriteReceived, p)
		},
	}

	input := []byte("abc")

	n, err := lw.Write(input)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Return value should be len(input), not len(filtered).
	if n != len(input) {
		t.Fatalf("Write n = %d, want %d", n, len(input))
	}

	// Underlying writer should receive the filtered bytes.
	if !bytes.Equal(writtenToUnderlying, []byte("XYc")) {
		t.Fatalf("underlying write = %q, want %q", writtenToUnderlying, "XYc")
	}

	// onWrite should receive the original (unfiltered) bytes.
	if !bytes.Equal(onWriteReceived, []byte("abc")) {
		t.Fatalf("onWrite received = %q, want %q", onWriteReceived, "abc")
	}
}

// --- Reassert / Quarantine tests ---

func newTestTerminalController() *TerminalController {
	tc := &TerminalController{
		width:  140,
		height: 40,
	}
	tc.lrMarginSupported.Store(true)
	tc.drawStatusBar = func() {}
	tc.setLastError = func(string) {}

	return tc
}

func TestScrollResetReassertsSidebar(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		tc.handleTerminalEvent(terminalEventScrollReset)
	})

	if !tc.SidebarEnabled() {
		t.Fatal("sidebar should remain enabled after single scroll reset")
	}

	if tc.sidebarForcedOff.Load() {
		t.Fatal("sidebarForcedOff should not be set")
	}

	if tc.sidebarQuarantined.Load() {
		t.Fatal("sidebarQuarantined should not be set after single event")
	}
}

func TestRepeatedScrollResetsQuarantineSidebar(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		for i := 0; i < tamperThreshold+1; i++ {
			tc.handleTerminalEvent(terminalEventScrollReset)
		}
	})

	if tc.SidebarEnabled() {
		t.Fatal("sidebar should be disabled after quarantine")
	}

	if tc.sidebarForcedOff.Load() {
		t.Fatal("sidebarForcedOff should NOT be set (quarantine is recoverable)")
	}

	if !tc.sidebarQuarantined.Load() {
		t.Fatal("sidebarQuarantined should be set")
	}
}

func TestQuarantineRecoveryViaToggle(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		// Trigger quarantine.
		for i := 0; i < tamperThreshold+1; i++ {
			tc.handleTerminalEvent(terminalEventScrollReset)
		}

		if !tc.sidebarQuarantined.Load() {
			t.Fatal("expected quarantine after repeated events")
		}

		// Toggle (^G) should recover.
		tc.toggleSidebar()
	})

	if tc.sidebarQuarantined.Load() {
		t.Fatal("quarantine should be cleared after toggle")
	}

	if !tc.SidebarEnabled() {
		t.Fatal("sidebar should be re-enabled after toggle recovery")
	}
}

func TestHardResetStillPermanentlyDisables(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		tc.handleTerminalEvent(terminalEventReset)
	})

	if !tc.sidebarForcedOff.Load() {
		t.Fatal("sidebarForcedOff should be set after hard reset")
	}

	// Toggle should be a no-op.
	captureStdout(t, func() {
		tc.toggleSidebar()
	})

	if !tc.sidebarForcedOff.Load() {
		t.Fatal("sidebarForcedOff should remain set after toggle attempt")
	}
}

func TestSoftResetPermanentlyDisables(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		tc.handleTerminalEvent(terminalEventSoftReset)
	})

	if !tc.sidebarForcedOff.Load() {
		t.Fatal("sidebarForcedOff should be set after soft reset")
	}
}

func TestTamperWindowResetsAfterTimeout(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		// Send events just below threshold.
		for i := 0; i < tamperThreshold-1; i++ {
			tc.handleTerminalEvent(terminalEventScrollReset)
		}

		// Simulate window expiry by resetting the start.
		tc.tamperWindowStart.Store(time.Now().Add(-3 * tamperWindow).UnixNano())

		// Next event should start a fresh window, not quarantine.
		tc.handleTerminalEvent(terminalEventScrollReset)
	})

	if tc.sidebarQuarantined.Load() {
		t.Fatal("quarantine should not be set after window expired")
	}

	if !tc.SidebarEnabled() {
		t.Fatal("sidebar should remain enabled")
	}
}

func TestDisableLREventReassertsSidebar(t *testing.T) {
	tc := newTestTerminalController()

	captureStdout(t, func() {
		tc.handleTerminalEvent(terminalEventDisableLR)
	})

	if !tc.SidebarEnabled() {
		t.Fatal("sidebar should remain enabled after single disable-LR event")
	}
}

func TestSidebarAvailableReflectsQuarantine(t *testing.T) {
	tc := newTestTerminalController()

	if !tc.SidebarAvailable() {
		t.Fatal("SidebarAvailable should be true initially")
	}

	tc.sidebarQuarantined.Store(true)

	if tc.SidebarAvailable() {
		t.Fatal("SidebarAvailable should be false when quarantined")
	}

	if tc.SidebarEnabled() {
		t.Fatal("SidebarEnabled should be false when quarantined")
	}

	tc.sidebarQuarantined.Store(false)
	tc.sidebarForcedOff.Store(true)

	if tc.SidebarAvailable() {
		t.Fatal("SidebarAvailable should be false when forcedOff")
	}
}

// --- EL (Erase in Line) filter tests ---

func TestSidebarFilterELRewrite(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		input  []byte
		want   []byte
	}{
		{
			name:   "CSI 2K rewritten when active",
			active: true,
			input:  []byte("\x1b[2K"),
			// paneDims: (2, 24, 38, 140) → paneW = 140 - 38 + 1 = 103
			// DECSC + CHA(38) + ECH(103) + DECRC
			want: []byte("\x1b7\x1b[38G\x1b[103X\x1b8"),
		},
		{
			name:   "CSI 1K rewritten when active",
			active: true,
			input:  []byte("\x1b[1K"),
			want:   []byte("\x1b7\x1b[38G\x1b[103X\x1b8"),
		},
		{
			name:   "CSI 0K passes through when active",
			active: true,
			input:  []byte("\x1b[0K"),
			want:   []byte("\x1b[0K"),
		},
		{
			name:   "bare CSI K (implicit mode 0) passes through when active",
			active: true,
			input:  []byte("\x1b[K"),
			// bare CSI K has no params, so it goes through the 'default' case in
			// the switch on third byte, not handleCSIParams. It passes through.
			want: []byte("\x1b[K"),
		},
		{
			name:   "CSI 2K passes through when inactive",
			active: false,
			input:  []byte("\x1b[2K"),
			want:   []byte("\x1b[2K"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sf := newTestFilter(tc.active)
			got := sf.rewrite(tc.input)

			if !bytes.Equal(got, tc.want) {
				t.Fatalf("rewrite(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSidebarFilterELChunkBoundary(t *testing.T) {
	sf := newTestFilter(true) // paneDims: (2, 24, 38, 140)

	// Split CSI 2K across two writes: ESC[2 | K
	got := sf.rewrite([]byte("\x1b[2"))
	got = append(got, sf.rewrite([]byte("K"))...)
	want := []byte("\x1b7\x1b[38G\x1b[103X\x1b8")

	if !bytes.Equal(got, want) {
		t.Fatalf("rewrite(CSI 2K split) = %q, want %q", got, want)
	}
}

func TestBuildPaneLineErase(t *testing.T) {
	got := buildPaneLineErase(38, 103)
	want := []byte("\x1b7\x1b[38G\x1b[103X\x1b8")

	if !bytes.Equal(got, want) {
		t.Fatalf("buildPaneLineErase(38, 103) = %q, want %q", got, want)
	}
}

// --- CSI J event detection tests ---

func TestParseTerminalEventsEraseDisplay(t *testing.T) {
	tests := []struct {
		name      string
		chunk     []byte
		wantEvent []terminalEvent
	}{
		{
			name:      "CSI 2J → erase-display",
			chunk:     []byte("\x1b[2J"),
			wantEvent: []terminalEvent{terminalEventEraseDisplay},
		},
		{
			name:      "bare CSI J → erase-display",
			chunk:     []byte("\x1b[J"),
			wantEvent: []terminalEvent{terminalEventEraseDisplay},
		},
		{
			name:      "CSI 0J → erase-display",
			chunk:     []byte("\x1b[0J"),
			wantEvent: []terminalEvent{terminalEventEraseDisplay},
		},
		{
			name:      "CSI 1J → erase-display",
			chunk:     []byte("\x1b[1J"),
			wantEvent: []terminalEvent{terminalEventEraseDisplay},
		},
		{
			name:      "CSI 3J → no event (scrollback erase is safe)",
			chunk:     []byte("\x1b[3J"),
			wantEvent: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, _ := parseTerminalEvents(nil, tc.chunk)
			if len(events) != len(tc.wantEvent) {
				t.Fatalf("events len = %d, want %d (%v)", len(events), len(tc.wantEvent), events)
			}

			for i := range events {
				if events[i] != tc.wantEvent[i] {
					t.Fatalf("events[%d] = %v, want %v", i, events[i], tc.wantEvent[i])
				}
			}
		})
	}
}

func TestTerminalEventNameEraseDisplay(t *testing.T) {
	got := terminalEventName(terminalEventEraseDisplay)
	if got != "erase-display" {
		t.Fatalf("terminalEventName(terminalEventEraseDisplay) = %q, want %q", got, "erase-display")
	}
}

func TestScheduleSidebarRedraw(t *testing.T) {
	var redrawCount atomic.Int32

	done := make(chan struct{}, 1)

	tc := &TerminalController{
		width:  140,
		height: 40,
	}
	tc.lrMarginSupported.Store(true)
	tc.drawStatusBar = func() {
		redrawCount.Add(1)

		select {
		case done <- struct{}{}:
		default:
		}
	}
	tc.setLastError = func(string) {}

	// Multiple rapid calls should coalesce.
	tc.scheduleSidebarRedraw()
	tc.scheduleSidebarRedraw()
	tc.scheduleSidebarRedraw()

	// Wait for the timer to fire with a bounded timeout.
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("sidebar redraw did not occur within 2s")
	}

	if got := redrawCount.Load(); got != 1 {
		t.Fatalf("expected 1 redraw, got %d", got)
	}
}
