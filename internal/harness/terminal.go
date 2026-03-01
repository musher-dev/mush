//go:build unix

package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/musher-dev/mush/internal/harness/ui/layout"
	"github.com/musher-dev/mush/internal/tui/ansi"
)

const maxTermSeqTailBytes = 16

const (
	tamperThreshold = 5               // max reassert events before quarantine
	tamperWindow    = 2 * time.Second // sliding window duration
)

type terminalEvent uint8

const (
	terminalEventReset terminalEvent = iota
	terminalEventSoftReset
	terminalEventScrollReset
	terminalEventDisableLR
	terminalEventAltEnter
	terminalEventAltExit
)

// TerminalController manages terminal I/O, scroll regions, sidebar toggling,
// resize handling, and alt-screen tracking.
type TerminalController struct {
	mu     sync.Mutex // serializes stdout writes
	width  int
	height int

	oldState *term.State

	lrMarginSupported  atomic.Bool
	forceSidebar       bool
	probeLeftoverInput []byte // user keystrokes captured during LR margin probe
	sidebarForcedOff   atomic.Bool
	sidebarQuarantined atomic.Bool // recoverable disable (rate limit exceeded)
	tamperCount        atomic.Int32
	tamperWindowStart  atomic.Int64 // UnixNano of window start
	sidebarUserOff     atomic.Bool
	altScreenActive    atomic.Bool
	termSeqTail        []byte
	termSeqMu          sync.Mutex
	sidebarDisableMu   sync.Mutex
	restoreOnce        sync.Once

	// Atomic frame dimensions for the output filter (Part 2).
	filterContentTop   atomic.Int32
	filterScrollBottom atomic.Int32

	// Set once in Run(), read-only thereafter.
	executors          map[string]Executor
	supportedHarnesses []string

	// Callbacks wired by RootModel.
	drawStatusBar func()
	setLastError  func(string)
	ptyActive     func() bool // true when any executor PTY is running (unsafe to probe)
}

// sidebarFilter is a stream filter for PTY output when the sidebar is active.
// It rewrites bare CSI s (SCOSC) → ESC 7 (DECSC), bare CSI u (SCORC) → ESC 8
// (DECRC), bare CSI r → CSI <top>;<bottom> r (preserve scroll region), and
// drops CSI ?69l (disable LR mode) and parameterized CSI Ps;Ps s (DECSLRM).
//
// The filter uses a tail buffer of up to 8 bytes to handle sequences split
// across Write chunk boundaries. If accumulated bytes exceed the buffer,
// they are flushed through (the onWrite callback still catches the event).
type sidebarFilter struct {
	active     func() bool       // true when sidebar is enabled (DECLRMM active)
	scrollDims func() (int, int) // returns (contentTop, scrollBottom) for CSI r rewrite
	tail       [8]byte           // held bytes from previous chunk
	tailN      int               // number of valid bytes in tail (0..8)
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

			top, bottom := sf.scrollDims()

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
			action, seqLen := sf.handleCSIParams(src[i:])
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
			} else if out != nil {
				out = append(out, src[i:i+seqLen]...)
			}

			i += seqLen

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
// Returns the action and total sequence length. seqLen=0 means incomplete.
func (sf *sidebarFilter) handleCSIParams(seq []byte) (action filterAction, seqLen int) {
	// seq starts with ESC [ <digit or ;>
	// Find the final byte (0x40-0x7E).
	for j := 2; j < len(seq); j++ {
		b := seq[j]
		if b >= 0x40 && b <= 0x7e {
			// Final byte found. Check for DECSLRM without string allocation.
			if b == 's' && isDECSLRMBytes(seq[2:j]) {
				return filterDrop, j + 1
			}

			return filterPass, j + 1
		}

		// Only digits and ; are valid parameter bytes.
		if (b < '0' || b > '9') && b != ';' {
			// Not a simple parameterized sequence — pass through.
			return filterPass, j + 1
		}
	}

	return filterPass, 0 // incomplete
}

// containsByte reports whether b contains the byte c.
func containsByte(b []byte, c byte) bool {
	return bytes.IndexByte(b, c) >= 0
}

// lockedWriter wraps an io.Writer with a mutex, an optional pre-write filter,
// and an optional onWrite callback.
type lockedWriter struct {
	mu      *sync.Mutex
	w       io.Writer
	filter  func([]byte) []byte // pre-write transform (nil = passthrough)
	onWrite func([]byte)
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()

	out := p
	if lw.filter != nil {
		out = lw.filter(p)
	}

	_, err := lw.w.Write(out)
	lw.mu.Unlock()

	// onWrite receives the original (unfiltered) bytes so that the event
	// parser can detect parameterized DECSLRM and other sequences.
	if lw.onWrite != nil && len(p) > 0 {
		lw.onWrite(p)
	}

	if err != nil {
		return len(p), fmt.Errorf("write to locked writer: %w", err)
	}

	return len(p), nil
}

// Write writes bytes to stdout under the terminal mutex.
func (tc *TerminalController) Write(p []byte) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	_, _ = os.Stdout.Write(p)
}

// WriteString writes a string to stdout under the terminal mutex.
func (tc *TerminalController) WriteString(s string) {
	tc.Write([]byte(s))
}

// Dimensions returns the current terminal width and height.
func (tc *TerminalController) Dimensions() (w, h int) {
	tc.mu.Lock()
	w = tc.width
	h = tc.height
	tc.mu.Unlock()

	return w, h
}

// SidebarEnabled returns true when the sidebar is active.
// Returns false during alt-screen mode so the output filter does not
// rewrite/drop sequences that full-screen TUI programs need.
func (tc *TerminalController) SidebarEnabled() bool {
	return tc.lrMarginSupported.Load() && !tc.sidebarForcedOff.Load() && !tc.sidebarQuarantined.Load() && !tc.sidebarUserOff.Load() && !tc.altScreenActive.Load()
}

// AltScreenActive returns true when the alt-screen buffer is active.
func (tc *TerminalController) AltScreenActive() bool {
	return tc.altScreenActive.Load()
}

// LRMarginSupported returns whether the terminal supports LR margins.
func (tc *TerminalController) LRMarginSupported() bool {
	return tc.lrMarginSupported.Load()
}

// SidebarAvailable returns whether the sidebar could be shown (LR supported, not force-disabled, not quarantined).
func (tc *TerminalController) SidebarAvailable() bool {
	return tc.lrMarginSupported.Load() && !tc.sidebarForcedOff.Load() && !tc.sidebarQuarantined.Load()
}

// setupScreen initializes the terminal with scroll region.
func (tc *TerminalController) setupScreen() {
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.filterContentTop.Store(int32(frame.ContentTop))
	tc.filterScrollBottom.Store(int32(frame.Height))
	tc.WriteString(layout.SetupSequence(frame, tc.SidebarEnabled()))
	tc.drawStatusBar()
}

// restore restores the terminal to its original state.
// Safe to call multiple times (from both defer and signal handler).
func (tc *TerminalController) restore() {
	tc.restoreOnce.Do(func() {
		tc.mu.Lock()
		h := tc.height
		seq := ansi.ResetScroll + ansi.DisableLRMode + ansi.ShowCursor + ansi.Reset + ansi.Move(h, 1) + "\n"
		_, _ = os.Stdout.WriteString(seq)
		tc.mu.Unlock()

		if tc.oldState != nil {
			_ = term.Restore(int(os.Stdin.Fd()), tc.oldState)
		}
	})
}

func (tc *TerminalController) detectLRMarginSupport() bool {
	if !supportsLRMargins(os.Getenv("TERM")) {
		return false
	}

	tc.mu.Lock()
	width := tc.width
	tc.mu.Unlock()

	supported, leftover := probeLRMarginSupport(os.Stdin, os.Stdout, lrMarginProbeTimeout, width)
	tc.probeLeftoverInput = leftover

	return supported
}

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
	}
}

func (tc *TerminalController) restoreLayoutAfterAltScreen() {
	tc.mu.Lock()
	sidebarOn := tc.SidebarEnabled()
	frame := layout.ComputeFrame(tc.width, tc.height, sidebarOn)

	seq := layout.ResizeSequenceWithCursor(frame, sidebarOn, false)
	if sidebarOn {
		// Wrap with SaveCursor/RestoreCursor to avoid DECSLRM cursor-homing side effect.
		seq = ansi.SaveCursor + seq + ansi.RestoreCursor
	}

	_, _ = fmt.Fprint(os.Stdout, seq)
	tc.mu.Unlock()

	tc.drawStatusBar()
}

func (tc *TerminalController) disableSidebar() {
	tc.sidebarDisableMu.Lock()
	if !tc.SidebarEnabled() {
		tc.sidebarDisableMu.Unlock()
		return
	}

	tc.mu.Lock()
	termWidth := tc.width
	oldFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.sidebarForcedOff.Store(true)
	newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())

	if oldFrame.SidebarVisible {
		_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequenceWithCursor(newFrame, tc.SidebarEnabled(), false))
	}
	tc.mu.Unlock()
	tc.sidebarDisableMu.Unlock()

	if !oldFrame.SidebarVisible {
		return
	}

	tc.clearSidebarArea(oldFrame.SidebarWidth+1, oldFrame.Height, termWidth)

	rows := layout.PtyRowsForFrame(newFrame)

	for _, executor := range tc.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.drawStatusBar()
}

// reassertOrQuarantine handles scroll-reset and disable-LR events with a
// sliding-window rate limiter. Single events reassert the layout; repeated
// events within the window quarantine the sidebar (recoverable via ^G).
func (tc *TerminalController) reassertOrQuarantine(ev terminalEvent) {
	if !tc.SidebarEnabled() {
		return
	}

	now := time.Now().UnixNano()
	winStart := tc.tamperWindowStart.Load()

	if now-winStart > tamperWindow.Nanoseconds() {
		// Window expired — reset.
		tc.tamperCount.Store(1)
		tc.tamperWindowStart.Store(now)
	} else {
		tc.tamperCount.Add(1)
	}

	if tc.tamperCount.Load() > int32(tamperThreshold) {
		tc.quarantineSidebar(ev)
		return
	}

	tc.reassertLayout()
	slog.Default().Debug(
		"sidebar layout reasserted after child event",
		slog.String("component", "harness"),
		slog.String("event.type", "sidebar.reassert"),
		slog.String("terminal_event", terminalEventName(ev)),
	)
}

// reassertLayout re-applies LR margins, scroll region, and status bar without
// tearing down the sidebar.
func (tc *TerminalController) reassertLayout() {
	tc.mu.Lock()
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	seq := ansi.SaveCursor +
		ansi.EnableLRMode +
		ansi.LRMargins(frame.PaneXStart, frame.Width) +
		ansi.ScrollRegion(frame.ContentTop, frame.Height) +
		ansi.RestoreCursor
	_, _ = fmt.Fprint(os.Stdout, seq)
	tc.mu.Unlock()

	tc.drawStatusBar()
}

// quarantineSidebar recovers the sidebar area and sets the quarantine flag.
// Unlike disableSidebar (permanent), quarantine is recoverable via ^G toggle.
func (tc *TerminalController) quarantineSidebar(ev terminalEvent) {
	tc.sidebarDisableMu.Lock()
	if !tc.SidebarEnabled() {
		tc.sidebarDisableMu.Unlock()
		return
	}

	tc.mu.Lock()
	termWidth := tc.width
	oldFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.sidebarQuarantined.Store(true)
	newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())

	if oldFrame.SidebarVisible {
		_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequenceWithCursor(newFrame, tc.SidebarEnabled(), false))
	}
	tc.mu.Unlock()
	tc.sidebarDisableMu.Unlock()

	if !oldFrame.SidebarVisible {
		return
	}

	tc.clearSidebarArea(oldFrame.SidebarWidth+1, oldFrame.Height, termWidth)

	rows := layout.PtyRowsForFrame(newFrame)

	for _, executor := range tc.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.setLastError("sidebar quarantined: repeated layout resets by child process (\x07 to retry)")

	slog.Default().Warn(
		"sidebar quarantined due to repeated layout-tamper events",
		slog.String("component", "harness"),
		slog.String("event.type", "sidebar.quarantine"),
		slog.String("terminal_event", terminalEventName(ev)),
		slog.Int("tamper_count", int(tc.tamperCount.Load())),
	)

	tc.drawStatusBar()
}

// terminalEventName returns a human-readable name for logging.
func terminalEventName(ev terminalEvent) string {
	switch ev {
	case terminalEventReset:
		return "hard-reset"
	case terminalEventSoftReset:
		return "soft-reset"
	case terminalEventScrollReset:
		return "scroll-reset"
	case terminalEventDisableLR:
		return "disable-lr"
	case terminalEventAltEnter:
		return "alt-enter"
	case terminalEventAltExit:
		return "alt-exit"
	default:
		return fmt.Sprintf("unknown(%d)", ev)
	}
}

// reprobeAndEnableSidebar re-runs the LR margin probe inline (called from
// copyInput via toggleSidebar when lrMarginSupported is false). If the probe
// succeeds, the sidebar is enabled and the layout is reconfigured.
func (tc *TerminalController) reprobeAndEnableSidebar() {
	// Unsafe to probe while any executor PTY is active — the probe reads
	// stdin and writes escape sequences that would corrupt the child process.
	// This covers both worker mode (job PTY running) and bundle-load mode
	// (interactive Claude PTY alive but no job loop).
	if tc.ptyActive != nil && tc.ptyActive() {
		return
	}

	supported := tc.detectLRMarginSupport()
	if !supported {
		return
	}

	tc.lrMarginSupported.Store(true)

	// Replay any keystrokes the user typed during the probe window.
	if len(tc.probeLeftoverInput) > 0 {
		leftover := tc.probeLeftoverInput
		tc.probeLeftoverInput = nil

		for _, harnessType := range tc.supportedHarnesses {
			if executor, ok := tc.executors[harnessType]; ok {
				if ir, ok := executor.(InputReceiver); ok {
					_, _ = ir.WriteInput(leftover)

					break
				}
			}
		}
	}

	tc.sidebarUserOff.Store(false)

	tc.sidebarDisableMu.Lock()
	tc.mu.Lock()

	newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequence(newFrame, tc.SidebarEnabled()))

	tc.mu.Unlock()
	tc.sidebarDisableMu.Unlock()

	rows := layout.PtyRowsForFrame(newFrame)

	for _, executor := range tc.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.drawStatusBar()
}

func (tc *TerminalController) toggleSidebar() {
	if tc.sidebarForcedOff.Load() || tc.altScreenActive.Load() {
		return
	}

	// Quarantine recovery: clear flag, reset tamper counters, re-enable.
	if tc.sidebarQuarantined.Load() {
		tc.sidebarQuarantined.Store(false)
		tc.tamperCount.Store(0)
		tc.tamperWindowStart.Store(0)

		tc.sidebarDisableMu.Lock()
		tc.mu.Lock()
		newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
		_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequence(newFrame, tc.SidebarEnabled()))
		tc.mu.Unlock()
		tc.sidebarDisableMu.Unlock()

		rows := layout.PtyRowsForFrame(newFrame)

		for _, executor := range tc.executors {
			if r, ok := executor.(Resizable); ok {
				r.Resize(rows, newFrame.PaneWidth)
			}
		}

		tc.drawStatusBar()

		return
	}

	if !tc.lrMarginSupported.Load() {
		tc.reprobeAndEnableSidebar()
		return
	}

	tc.sidebarDisableMu.Lock()

	tc.mu.Lock()
	oldFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	termWidth := tc.width

	tc.sidebarUserOff.Store(!tc.sidebarUserOff.Load())

	newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequence(newFrame, tc.SidebarEnabled()))
	tc.mu.Unlock()
	tc.sidebarDisableMu.Unlock()

	if oldFrame.SidebarVisible && !newFrame.SidebarVisible {
		tc.clearSidebarArea(oldFrame.SidebarWidth+1, oldFrame.Height, termWidth)
	}

	rows := layout.PtyRowsForFrame(newFrame)

	for _, executor := range tc.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.drawStatusBar()
}

func (tc *TerminalController) readTerminalSize() (width, height int, err error) {
	width, height, err = term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return 0, 0, fmt.Errorf("get terminal size: %w", err)
	}

	width, height = clampTerminalSize(width, height)

	return width, height, nil
}

func (tc *TerminalController) resizeLoop(ctx context.Context, done <-chan struct{}) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(ResizePollInterval)
	defer ticker.Stop()

	tc.refreshTerminalSize()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-sigCh:
			tc.refreshTerminalSize()
		case <-ticker.C:
			tc.refreshTerminalSize()
		}
	}
}

func (tc *TerminalController) refreshTerminalSize() {
	width, height, err := tc.readTerminalSize()
	if err != nil {
		tc.setLastError(fmt.Sprintf("Terminal resize read failed: %v", err))
		return
	}

	tc.handleResize(width, height)
}

func (tc *TerminalController) handleResize(width, height int) {
	width, height = clampTerminalSize(width, height)

	if tc.altScreenActive.Load() {
		tc.mu.Lock()
		tc.width = width
		tc.height = height
		tc.mu.Unlock()

		frame := layout.ComputeFrame(width, height, false)
		rows := layout.PtyRowsForFrame(frame)

		for _, executor := range tc.executors {
			if r, ok := executor.(Resizable); ok {
				r.Resize(rows, frame.PaneWidth)
			}
		}

		return
	}

	tc.mu.Lock()
	if width == tc.width && height == tc.height {
		tc.mu.Unlock()
		return
	}

	oldFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.width = width
	tc.height = height
	newFrame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.filterContentTop.Store(int32(newFrame.ContentTop))
	tc.filterScrollBottom.Store(int32(newFrame.Height))
	_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequenceWithCursor(newFrame, tc.SidebarEnabled(), false))
	tc.mu.Unlock()

	if oldFrame.SidebarVisible && !newFrame.SidebarVisible {
		tc.clearSidebarArea(oldFrame.SidebarWidth+1, height, width)
	} else if oldFrame.SidebarVisible && newFrame.SidebarVisible && oldFrame.SidebarWidth != newFrame.SidebarWidth {
		clearWidth := oldFrame.SidebarWidth
		if newFrame.SidebarWidth > clearWidth {
			clearWidth = newFrame.SidebarWidth
		}

		tc.clearSidebarArea(clearWidth+1, height, width)
	}

	rows := layout.PtyRowsForFrame(newFrame)

	// Resize all Resizable executors.
	for _, executor := range tc.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.drawStatusBar()
}

func (tc *TerminalController) clearSidebarArea(columns, height, termWidth int) {
	if columns <= 0 {
		return
	}

	if columns > termWidth {
		columns = termWidth
	}

	rows := height - layout.TopBarHeight
	if rows < 1 {
		return
	}

	var b strings.Builder

	blank := strings.Repeat(" ", columns)

	for i := 0; i < rows; i++ {
		b.WriteString(ansi.Move(layout.TopBarHeight+1+i, 1))
		b.WriteString(blank)
	}

	tc.WriteString(b.String())
}

func clampTerminalSize(width, height int) (clampedWidth, clampedHeight int) {
	return layout.ClampTerminalSize(width, height)
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
