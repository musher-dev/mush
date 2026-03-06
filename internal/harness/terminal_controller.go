//go:build unix

package harness

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
	"github.com/musher-dev/mush/internal/tui/ansi"
)

const (
	tamperThreshold    = 5               // max reassert events before quarantine
	tamperWindow       = 2 * time.Second // sliding window duration
	sidebarRedrawDelay = 16 * time.Millisecond
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
	altScreenActive    atomic.Bool
	termSeqTail        []byte
	termSeqMu          sync.Mutex
	sidebarDisableMu   sync.Mutex
	sidebarRedrawMu    sync.Mutex
	sidebarRedrawTimer *time.Timer
	restoreOnce        sync.Once

	// Atomic frame dimensions for the output filter.
	filterContentTop   atomic.Int32
	filterScrollBottom atomic.Int32
	filterPaneXStart   atomic.Int32
	filterTermWidth    atomic.Int32

	// Set once in Run(), read-only thereafter.
	executors          map[string]harnesstype.Executor
	supportedHarnesses []string

	// Callbacks wired by RootModel.
	drawStatusBar func()
	setLastError  func(string)
	ptyActive     func() bool // true when any executor PTY is running (unsafe to probe)
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
	return tc.lrMarginSupported.Load() && !tc.sidebarForcedOff.Load() && !tc.sidebarQuarantined.Load() && !tc.altScreenActive.Load()
}

// AltScreenActive returns true when the alt-screen buffer is active.
func (tc *TerminalController) AltScreenActive() bool {
	return tc.altScreenActive.Load()
}

// LRMarginSupported returns whether the terminal supports LR margins.
func (tc *TerminalController) LRMarginSupported() bool {
	return tc.lrMarginSupported.Load()
}

// storeFilterDims updates the atomic frame dimensions read by the sidebarFilter.
// Must be called on every layout transition that changes pane geometry so that
// cursor-coordinate translation (CUP/CHA/HPA/VPA) uses the correct offsets.
func (tc *TerminalController) storeFilterDims(frame layout.Frame) {
	tc.filterContentTop.Store(int32(frame.ContentTop))
	tc.filterScrollBottom.Store(int32(frame.Height))
	tc.filterPaneXStart.Store(int32(frame.PaneXStart))
	tc.filterTermWidth.Store(int32(frame.Width))
}

// setupScreen initializes the terminal with scroll region.
func (tc *TerminalController) setupScreen() {
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.storeFilterDims(frame)
	tc.WriteString(layout.SetupSequence(frame, tc.SidebarEnabled()))
	tc.drawStatusBar()
}

// restore restores the terminal to its original state.
// Safe to call multiple times (from both defer and signal handler).
func (tc *TerminalController) restore() {
	tc.restoreOnce.Do(func() {
		// Stop any pending sidebar redraw timer so it doesn't fire after
		// the terminal has been reset.
		tc.sidebarRedrawMu.Lock()
		if tc.sidebarRedrawTimer != nil {
			tc.sidebarRedrawTimer.Stop()
			tc.sidebarRedrawTimer = nil
		}
		tc.sidebarRedrawMu.Unlock()

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

func (tc *TerminalController) restoreLayoutAfterAltScreen() {
	tc.mu.Lock()
	sidebarOn := tc.SidebarEnabled()
	frame := layout.ComputeFrame(tc.width, tc.height, sidebarOn)
	tc.storeFilterDims(frame)

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
		if r, ok := executor.(harnesstype.Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.drawStatusBar()
}

// reassertOrQuarantine handles scroll-reset and disable-LR events with a
// fixed-window rate limiter. Single events reassert the layout; repeated
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
// tearing down the sidebar. Only emits LR-mode sequences when the sidebar is
// actually visible (terminal wide enough); otherwise just restores the scroll region.
func (tc *TerminalController) reassertLayout() {
	tc.mu.Lock()
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	tc.storeFilterDims(frame)

	seq := ansi.SaveCursor
	if frame.SidebarVisible {
		seq += ansi.EnableLRMode +
			ansi.LRMargins(frame.PaneXStart, frame.Width)
	}

	seq += ansi.ScrollRegion(frame.ContentTop, frame.Height) +
		ansi.RestoreCursor
	_, _ = fmt.Fprint(os.Stdout, seq)
	tc.mu.Unlock()

	tc.drawStatusBar()
}

// scheduleSidebarRedraw triggers a debounced sidebar redraw after CSI J
// (Erase in Display) is detected. Multiple rapid calls coalesce into a single
// redraw after sidebarRedrawDelay.
func (tc *TerminalController) scheduleSidebarRedraw() {
	tc.sidebarRedrawMu.Lock()
	defer tc.sidebarRedrawMu.Unlock()

	if tc.sidebarRedrawTimer != nil {
		return // already scheduled
	}

	tc.sidebarRedrawTimer = time.AfterFunc(sidebarRedrawDelay, func() {
		tc.sidebarRedrawMu.Lock()
		tc.sidebarRedrawTimer = nil
		tc.sidebarRedrawMu.Unlock()

		if tc.SidebarEnabled() {
			tc.drawStatusBar()
		}
	})
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
		if r, ok := executor.(harnesstype.Resizable); ok {
			r.Resize(rows, newFrame.PaneWidth)
		}
	}

	tc.setLastError("sidebar quarantined: repeated layout resets by child process")

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
	case terminalEventEraseDisplay:
		return "erase-display"
	default:
		return fmt.Sprintf("unknown(%d)", ev)
	}
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
			if r, ok := executor.(harnesstype.Resizable); ok {
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
	tc.storeFilterDims(newFrame)
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

	// Resize all harnesstype.Resizable executors.
	for _, executor := range tc.executors {
		if r, ok := executor.(harnesstype.Resizable); ok {
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
