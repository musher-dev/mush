//go:build unix

package harness

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	mu     sync.Mutex // serializes stdout writes (was termMu)
	width  int
	height int

	oldState *term.State

	lrMarginSupported  atomic.Bool
	forceSidebar       bool
	probeLeftoverInput []byte // user keystrokes captured during LR margin probe
	sidebarForcedOff   atomic.Bool
	sidebarUserOff     atomic.Bool
	altScreenActive    atomic.Bool
	termSeqTail        []byte
	termSeqMu          sync.Mutex
	sidebarDisableMu   sync.Mutex
	restoreOnce        sync.Once

	// Set once in Run(), read-only thereafter.
	executors          map[string]Executor
	supportedHarnesses []string

	// Callbacks wired by RootModel.
	drawStatusBar func()
	setLastError  func(string)
}

// lockedWriter wraps an io.Writer with a mutex and an optional onWrite callback.
type lockedWriter struct {
	mu      *sync.Mutex
	w       io.Writer
	onWrite func([]byte)
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	written, err := lw.w.Write(p)
	lw.mu.Unlock()

	if lw.onWrite != nil && written > 0 {
		lw.onWrite(p[:written])
	}

	if err != nil {
		return written, fmt.Errorf("write to locked writer: %w", err)
	}

	return written, nil
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
func (tc *TerminalController) SidebarEnabled() bool {
	return tc.lrMarginSupported.Load() && !tc.sidebarForcedOff.Load() && !tc.sidebarUserOff.Load()
}

// AltScreenActive returns true when the alt-screen buffer is active.
func (tc *TerminalController) AltScreenActive() bool {
	return tc.altScreenActive.Load()
}

// LRMarginSupported returns whether the terminal supports LR margins.
func (tc *TerminalController) LRMarginSupported() bool {
	return tc.lrMarginSupported.Load()
}

// SidebarAvailable returns whether the sidebar could be shown (LR supported and not force-disabled).
func (tc *TerminalController) SidebarAvailable() bool {
	return tc.lrMarginSupported.Load() && !tc.sidebarForcedOff.Load()
}

// setupScreen initializes the terminal with scroll region.
func (tc *TerminalController) setupScreen() {
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
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

	supported, leftover := probeLRMarginSupport(os.Stdin, os.Stdout, lrMarginProbeTimeout, tc.width)
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
	case terminalEventReset, terminalEventSoftReset, terminalEventScrollReset, terminalEventDisableLR:
		tc.disableSidebar()
	}
}

func (tc *TerminalController) restoreLayoutAfterAltScreen() {
	tc.mu.Lock()
	frame := layout.ComputeFrame(tc.width, tc.height, tc.SidebarEnabled())
	_, _ = fmt.Fprint(os.Stdout, layout.ResizeSequenceWithCursor(frame, tc.SidebarEnabled(), false))
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

// reprobeAndEnableSidebar re-runs the LR margin probe inline (called from
// copyInput via toggleSidebar when lrMarginSupported is false). If the probe
// succeeds, the sidebar is enabled and the layout is reconfigured.
func (tc *TerminalController) reprobeAndEnableSidebar(currentJobID func() string) {
	// Unsafe to probe while a job's PTY is active — the probe reads stdin
	// and writes escape sequences that would corrupt the child process.
	if currentJobID() != "" {
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

func (tc *TerminalController) toggleSidebar(currentJobID func() string) {
	if tc.sidebarForcedOff.Load() || tc.altScreenActive.Load() {
		return
	}

	if !tc.lrMarginSupported.Load() {
		tc.reprobeAndEnableSidebar(currentJobID)
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
