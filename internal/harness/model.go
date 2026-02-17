//go:build unix

package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"golang.org/x/term"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/linking"
	"github.com/musher-dev/mush/internal/transcript"
)

// SignalFileName is the marker file created by the Stop hook.
const SignalFileName = "complete"

// PromptDetectionBytes contains the bytes to detect Claude's input prompt.
// We look for "❯ " (U+276F HEAVY RIGHT-POINTING ANGLE QUOTATION MARK ORNAMENT + space)
// to know Claude is ready for input (used for initial ready state).
var PromptDetectionBytes = []byte{0xe2, 0x9d, 0xaf, 0x20} // "❯ " in UTF-8

// PromptDebounceTime is how long to wait after seeing the prompt before
// declaring Claude is ready. Used only for initial startup detection.
const PromptDebounceTime = 1 * time.Second

// SignalPollInterval is how often to check for completion signal files.
const SignalPollInterval = 200 * time.Millisecond

// ResizePollInterval is the fallback interval to reconcile terminal size in
// environments that don't reliably forward SIGWINCH.
const ResizePollInterval = 250 * time.Millisecond

// PTYWriteChunkSize is the max bytes to write to the PTY at once.
const PTYWriteChunkSize = 4096

// PTYChunkDelay is the delay between writing chunks to the PTY.
const PTYChunkDelay = 10 * time.Millisecond

// PTYPostWriteDelay is the delay after writing all content before sending Enter.
const PTYPostWriteDelay = 50 * time.Millisecond

// PTYPasteSettleDelay is the delay after bulk-pasting content, allowing the
// application to process the pasted text before we send Enter.
const PTYPasteSettleDelay = 500 * time.Millisecond

// DefaultExecutionTimeout is the fallback when no execution timeout is set on the job.
const DefaultExecutionTimeout = 10 * time.Minute

// ANSI escape sequences for terminal control
const (
	escClearScreen   = "\x1b[2J"
	escMoveTo        = "\x1b[%d;%dH" // row;col (1-indexed)
	escSaveCursor    = "\x1b[s"
	escRestoreCursor = "\x1b[u"
	escSetScrollRgn  = "\x1b[%d;%dr" // top;bottom
	escResetScroll   = "\x1b[r"
	escReset         = "\x1b[0m"
	escShowCursor    = "\x1b[?25h"
	escClearLine     = "\x1b[2K"
)

// StatusBarHeight is the number of lines reserved for the status bar
const StatusBarHeight = 2

const (
	ctrlQ = 0x11
	ctrlS = 0x13
	ctrlC = 0x03
	esc   = 0x1b
)

const (
	defaultCtrlCExitWindow     = 2 * time.Second
	defaultPTYShutdownDeadline = 3 * time.Second
)

// RootModel manages the harness state with scroll region approach.
type RootModel struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Serializes all writes to the terminal (stdout/stderr) to avoid cursor-control
	// sequences interleaving with job output.
	termMu sync.Mutex

	// Terminal dimensions
	width  int
	height int

	// PTY management
	ptyMu sync.Mutex
	ptmx  *os.File
	cmd   *exec.Cmd
	// cmdPGID is the process-group ID for the active Claude process.
	cmdPGID int
	// setPTYSize is injectable for tests; defaults to pty.Setsize.
	setPTYSize func(*os.File, *pty.Winsize) error
	// startPTYWithSize is injectable for tests; defaults to pty.StartWithSize.
	startPTYWithSize func(*exec.Cmd, *pty.Winsize) (*os.File, error)
	// ptyReady delivers active PTY handles to the output reader loop.
	ptyReady chan *os.File

	// Transcript capture for session history.
	transcriptStore *transcript.Store
	transcriptMu    sync.Mutex

	// Input mode state.
	inputMu        sync.Mutex
	copyMode       bool
	copyEscPending bool
	lastCtrlCAt    time.Time

	// Status bar state
	statusMu      sync.Mutex
	status        ConnectionStatus
	lastHeartbeat time.Time
	completed     int
	failed        int

	// Configuration
	client             *client.Client
	cfg                *config.Config
	habitatID          string
	queueID            string
	supportedHarnesses []string
	instanceID         string
	linkID             string
	signalDir          string
	restoreHooks       func() error
	runnerConfig       *client.RunnerConfigResponse
	mcpConfigPath      string
	mcpConfigSig       string
	mcpConfigRemove    func() error
	loadedMCPNames     []string
	transcriptEnabled  bool
	transcriptDir      string
	transcriptLines    int

	refreshMu              sync.Mutex
	refreshInterval        time.Duration
	pendingRunnerConfig    *client.RunnerConfigResponse
	pendingRunnerConfigSig string
	claudeRestartNeeded    bool

	// Original terminal state for restoration
	oldState *term.State

	// Control channels
	done      chan struct{}
	closeOnce sync.Once // Ensures done channel is closed only once

	// Job lifecycle management
	jobMu           sync.Mutex
	outputBuffer    bytes.Buffer  // Captures output during job execution
	capturing       bool          // True when we're capturing output for a job
	currentJob      *client.Job   // Currently executing job
	promptDetected  chan struct{} // Signals that Claude's prompt was detected
	readyForJob     bool          // True when Claude is ready for input (prompt visible)
	lastPromptSeen  time.Time     // When we last saw the prompt pattern
	promptConfirmed bool          // True after debounce confirms prompt
	bypassAccepted  bool          // True after we've auto-accepted bypass confirmation
	lastError       string        // Last error message
	lastErrorTime   time.Time     // Time of the last error
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc

	// Time and lifecycle behavior knobs (defaulted in constructor; injectable in tests).
	now                 func() time.Time
	ctrlCExitWindow     time.Duration
	ptyShutdownDeadline time.Duration
	killProcess         func(int, syscall.Signal) error
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

func (m *RootModel) termWrite(p []byte) {
	m.termMu.Lock()
	defer m.termMu.Unlock()
	_, _ = os.Stdout.Write(p)
}

func (m *RootModel) termWriteString(s string) {
	m.termWrite([]byte(s))
}

func (m *RootModel) termPrintf(format string, args ...any) {
	m.termMu.Lock()
	defer m.termMu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, format, args...)
}

// NewRootModel creates a new root model with the given configuration.
func NewRootModel(ctx context.Context, cfg *Config) *RootModel {
	ctx, cancel := context.WithCancel(ctx)

	return &RootModel{
		ctx:                 ctx,
		cancel:              cancel,
		status:              StatusConnecting,
		lastHeartbeat:       time.Now(),
		client:              cfg.Client,
		cfg:                 config.Load(),
		habitatID:           cfg.HabitatID,
		queueID:             cfg.QueueID,
		supportedHarnesses:  cfg.SupportedHarnesses,
		instanceID:          cfg.InstanceID,
		runnerConfig:        cfg.RunnerConfig,
		refreshInterval:     normalizeRefreshInterval(0),
		done:                make(chan struct{}),
		promptDetected:      make(chan struct{}, 1),
		ptyReady:            make(chan *os.File, 4),
		setPTYSize:          pty.Setsize,
		startPTYWithSize:    pty.StartWithSize,
		transcriptEnabled:   cfg.TranscriptEnabled,
		transcriptDir:       cfg.TranscriptDir,
		transcriptLines:     cfg.TranscriptLines,
		now:                 time.Now,
		ctrlCExitWindow:     defaultCtrlCExitWindow,
		ptyShutdownDeadline: defaultPTYShutdownDeadline,
		killProcess:         syscall.Kill,
	}
}

// signalDone safely closes the done channel exactly once.
func (m *RootModel) signalDone() {
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

// Run starts the harness with scroll region approach.
func (m *RootModel) Run() error {
	if m.client == nil {
		return fmt.Errorf("missing client in harness config")
	}

	// Get terminal size
	width, height, err := m.readTerminalSize()
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}
	m.width = width
	m.height = height

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	m.oldState = oldState
	defer m.restore()

	// Clear screen and set up scroll region
	m.setupScreen()

	historyEnabled := m.transcriptEnabled
	if !historyEnabled {
		historyEnabled = m.cfg.HistoryEnabled()
	}
	if historyEnabled && m.shouldCaptureTranscript() {
		historyDir := m.transcriptDir
		if historyDir == "" {
			historyDir = m.cfg.HistoryDir()
		}
		historyLines := m.transcriptLines
		if historyLines <= 0 {
			historyLines = m.cfg.HistoryLines()
		}
		sessionID := uuid.NewString()
		store, tErr := transcript.NewStore(transcript.StoreOptions{
			SessionID: sessionID,
			Dir:       historyDir,
			MaxLines:  historyLines,
		})
		if tErr != nil {
			m.setLastError(fmt.Sprintf("Transcript disabled: %v", tErr))
		} else {
			m.transcriptMu.Lock()
			m.transcriptStore = store
			m.transcriptMu.Unlock()
			defer m.closeTranscript()
		}
	}

	// Claude-specific wiring (PTY + stop-hook completion).
	// If the harness is running without the Claude harness, we skip all Claude setup
	// and only run non-Claude jobs (e.g. bash) in the scroll region.
	if m.isHarnessSupported("claude") {
		// Create per-run signal directory
		signalDir, mkErr := os.MkdirTemp("", "mush-signals-")
		if mkErr != nil {
			return fmt.Errorf("failed to create signal directory: %w", mkErr)
		}
		m.signalDir = signalDir
		defer func() {
			_ = os.RemoveAll(signalDir)
		}()

		// Install Stop hook for completion signaling
		restoreHooks, hookErr := installStopHook(signalDir)
		if hookErr != nil {
			return hookErr
		}
		m.restoreHooks = restoreHooks
		defer func() {
			if m.restoreHooks != nil {
				_ = m.restoreHooks()
			}
		}()

		// Build an ephemeral Claude MCP config from runner config, if available.
		if mcpErr := m.applyRunnerConfigForClaude(m.runnerConfig); mcpErr != nil {
			m.setLastError(fmt.Sprintf("MCP config disabled: %v", mcpErr))
		}
		defer m.cleanupMCPConfigFile()
	}

	// Register link with the platform
	name, metadata := linking.DefaultLinkInfo()
	linkID, err := linking.Register(m.ctx, m.client, m.habitatID, m.instanceID, name, metadata, buildinfo.Version)
	if err != nil {
		return fmt.Errorf("failed to register link: %w", err)
	}
	m.linkID = linkID

	linkHeartbeatCtx, cancelLinkHeartbeat := context.WithCancel(m.ctx)
	defer cancelLinkHeartbeat()
	linking.StartHeartbeat(linkHeartbeatCtx, m.client, m.linkID, m.currentJobID, func(err error) {
		m.setLastError(fmt.Sprintf("Link heartbeat failed: %v", err))
	})
	defer func() {
		m.statusMu.Lock()
		completed := m.completed
		failed := m.failed
		m.statusMu.Unlock()
		if err := linking.Deregister(m.client, m.linkID, completed, failed); err != nil {
			m.setLastError(fmt.Sprintf("Link deregistration failed: %v", err))
		}
	}()

	// Start Claude Code in PTY if it's a supported harness.
	if m.isHarnessSupported("claude") {
		if err := m.startPTY(); err != nil {
			return fmt.Errorf("failed to start PTY: %w", err)
		}
		defer m.closePTY()
	}

	// Start goroutines
	var wg sync.WaitGroup

	// Terminal resize watcher (SIGWINCH + periodic reconciliation).
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.resizeLoop()
	}()

	// PTY output -> terminal (in scroll region)
	if m.isHarnessSupported("claude") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.copyPTYOutput()
		}()
	}

	// Stdin -> PTY (with quit key handling). If no PTY is running, we still
	// consume Ctrl+Q so the user can exit the watch UI.
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.copyInput()
	}()

	// Status bar updater
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.updateStatusLoop()
	}()

	// Job manager (polls for and processes jobs)
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.jobManagerLoop()
	}()

	// Runner config refresh loop for MCP credential rotation.
	if m.isHarnessSupported("claude") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.runnerConfigRefreshLoop()
		}()
	}

	// Ensure external context cancellation (SIGTERM/SIGINT at command layer) can
	// always unblock Run() even when local input keys are not pressed.
	go func() {
		select {
		case <-m.ctx.Done():
			m.signalDone()
		case <-m.done:
		}
	}()

	// Wait for done signal
	<-m.done

	// Give goroutines time to finish
	m.cancel()
	wg.Wait()

	return nil
}

// isHarnessSupported checks if a given harness type is in the supported list.
func (m *RootModel) isHarnessSupported(harnessType string) bool {
	for _, a := range m.supportedHarnesses {
		if a == harnessType {
			return true
		}
	}
	return false
}

// setupScreen initializes the terminal with scroll region.
func (m *RootModel) setupScreen() {
	// Clear screen
	m.termWriteString(escClearScreen)

	// Draw initial status bar
	m.drawStatusBar()

	// Set scroll region (line StatusBarHeight+1 to bottom)
	scrollStart := StatusBarHeight + 1
	m.termPrintf(escSetScrollRgn, scrollStart, m.height)

	// Move cursor to scroll region
	m.termPrintf(escMoveTo, scrollStart, 1)
}

func (m *RootModel) shouldCaptureTranscript() bool {
	return m.isHarnessSupported("claude")
}

func (m *RootModel) appendTranscript(stream string, chunk []byte) {
	m.transcriptMu.Lock()
	store := m.transcriptStore
	m.transcriptMu.Unlock()
	if store == nil || len(chunk) == 0 {
		return
	}
	if err := store.Append(stream, chunk); err != nil {
		m.setLastError(fmt.Sprintf("Transcript write failed: %v", err))
	}
}

func (m *RootModel) closeTranscript() {
	m.transcriptMu.Lock()
	store := m.transcriptStore
	m.transcriptStore = nil
	m.transcriptMu.Unlock()
	if store == nil {
		return
	}
	if err := store.Close(); err != nil {
		m.setLastError(fmt.Sprintf("Transcript close failed: %v", err))
	}
}

func (m *RootModel) setCopyMode(enabled bool) {
	m.inputMu.Lock()
	changed := m.copyMode != enabled
	m.copyMode = enabled
	if !enabled {
		m.copyEscPending = false
	}
	m.inputMu.Unlock()
	if changed {
		m.drawStatusBar()
	}
}

func (m *RootModel) isCopyMode() bool {
	m.inputMu.Lock()
	defer m.inputMu.Unlock()
	return m.copyMode
}

func (m *RootModel) setCopyEscPending(pending bool) {
	m.inputMu.Lock()
	m.copyEscPending = pending
	m.inputMu.Unlock()
}

func (m *RootModel) popCopyEscPending() bool {
	m.inputMu.Lock()
	defer m.inputMu.Unlock()
	pending := m.copyEscPending
	m.copyEscPending = false
	return pending
}

func clampTerminalSize(width, height int) (clampedWidth, clampedHeight int) {
	if width < 20 {
		width = 20
	}
	minHeight := StatusBarHeight + 1
	if height < minHeight {
		height = minHeight
	}
	return width, height
}

func (m *RootModel) ptyRowsForHeight(height int) int {
	rows := height - StatusBarHeight
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *RootModel) readTerminalSize() (width, height int, err error) {
	width, height, err = term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return 0, 0, err
	}
	width, height = clampTerminalSize(width, height)
	return width, height, nil
}

func (m *RootModel) resizeLoop() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(ResizePollInterval)
	defer ticker.Stop()

	m.refreshTerminalSize()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		case <-sigCh:
			m.refreshTerminalSize()
		case <-ticker.C:
			m.refreshTerminalSize()
		}
	}
}

func (m *RootModel) refreshTerminalSize() {
	width, height, err := m.readTerminalSize()
	if err != nil {
		m.setLastError(fmt.Sprintf("Terminal resize read failed: %v", err))
		return
	}
	m.handleResize(width, height)
}

func (m *RootModel) handleResize(width, height int) {
	width, height = clampTerminalSize(width, height)

	m.termMu.Lock()
	if width == m.width && height == m.height {
		m.termMu.Unlock()
		return
	}
	m.width = width
	m.height = height
	scrollStart := StatusBarHeight + 1
	_, _ = fmt.Fprintf(os.Stdout, escSetScrollRgn, scrollStart, m.height)
	_, _ = fmt.Fprintf(os.Stdout, escMoveTo, scrollStart, 1)
	m.termMu.Unlock()

	_ = m.resizeActivePTY(width, m.ptyRowsForHeight(height))
	m.drawStatusBar()
}

func (m *RootModel) resizeActivePTY(cols, rows int) error {
	m.ptyMu.Lock()
	ptmx := m.ptmx
	m.ptyMu.Unlock()
	if ptmx == nil {
		return nil
	}

	setSize := m.setPTYSize
	if setSize == nil {
		setSize = pty.Setsize
	}

	if err := setSize(ptmx, &pty.Winsize{
		Rows: uint16(rows), //nolint:gosec // G115: terminal dimensions bounded by guardrails
		Cols: uint16(cols), //nolint:gosec // G115: terminal dimensions bounded by guardrails
	}); err != nil {
		m.setLastError(fmt.Sprintf("PTY resize failed: %v", err))
		return err
	}
	return nil
}

// drawStatusBar renders the status bar at the top of the screen.
func (m *RootModel) drawStatusBar() {
	m.statusMu.Lock()
	habitatID := m.habitatID
	queueID := m.queueID
	status := m.status
	completed := m.completed
	failed := m.failed
	lastHeartbeat := m.lastHeartbeat
	lastError := m.lastError
	lastErrorTime := m.lastErrorTime
	m.statusMu.Unlock()

	m.jobMu.Lock()
	jobID := ""
	if m.currentJob != nil {
		jobID = m.currentJob.ID
	}
	m.jobMu.Unlock()

	hbAge := formatHeartbeatAge(lastHeartbeat)
	renderedStatus := renderStatus(status)

	// Save cursor and move to top
	var b strings.Builder
	b.WriteString(escSaveCursor)
	b.WriteString(fmt.Sprintf(escMoveTo, 1, 1))

	// Line 1: MUSH HARNESS | Habitat | Status | Job
	line1Parts := []string{
		"\x1b[1mMUSH HARNESS\x1b[0m",
		fmt.Sprintf("Habitat: \x1b[1m%s\x1b[0m", habitatID),
		fmt.Sprintf("Status: %s", renderedStatus),
	}
	if m.isCopyMode() {
		line1Parts = append(line1Parts, "Mode: \x1b[33mCOPY\x1b[0m")
	} else {
		line1Parts = append(line1Parts, "Mode: \x1b[32mLIVE\x1b[0m")
	}

	if jobID != "" {
		line1Parts = append(line1Parts, fmt.Sprintf("Job: \x1b[1m%s\x1b[0m", jobID))
	} else {
		line1Parts = append(line1Parts, "\x1b[90mJob: (waiting...)\x1b[0m")
	}
	line1 := strings.Join(line1Parts, " \x1b[90m|\x1b[0m ")

	// Render line 1
	b.WriteString(escClearLine)
	b.WriteString("\x1b[48;5;236m\x1b[38;5;252m ")
	b.WriteString(line1)
	padding := m.width - m.visibleLength(line1) - 2
	if padding > 0 {
		b.WriteString(strings.Repeat(" ", padding))
	}
	b.WriteString(" \x1b[0m")

	// Move to line 2
	b.WriteString(fmt.Sprintf(escMoveTo, 2, 1))

	// Line 2: Heartbeat | Queue | Completed | Failed | Last Error
	line2Parts := []string{
		fmt.Sprintf("HB: \x1b[1m%s\x1b[0m", hbAge),
		fmt.Sprintf("Queue ID: \x1b[1m%s\x1b[0m", queueID),
		fmt.Sprintf("Done: \x1b[1m%d\x1b[0m", completed),
		fmt.Sprintf("Failed: \x1b[1m%d\x1b[0m", failed),
	}
	if lastError != "" && time.Since(lastErrorTime) < 30*time.Second {
		errorStr := lastError
		if len(errorStr) > 40 {
			errorStr = errorStr[:37] + "..."
		}
		line2Parts = append(line2Parts, fmt.Sprintf("Error: \x1b[31m%s\x1b[0m", errorStr))
	}

	line2 := strings.Join(line2Parts, " \x1b[90m|\x1b[0m ")

	// Render line 2
	b.WriteString(escClearLine)
	b.WriteString("\x1b[48;5;236m\x1b[38;5;252m ")
	b.WriteString(line2)
	padding = m.width - m.visibleLength(line2) - 2
	if padding > 0 {
		b.WriteString(strings.Repeat(" ", padding))
	}
	b.WriteString(" \x1b[0m")

	// Restore cursor
	b.WriteString(escRestoreCursor)

	m.termWriteString(b.String())
}

func renderStatus(status ConnectionStatus) string {
	switch status {
	case StatusConnected:
		return "\x1b[32m\x1b[1mConnected\x1b[0m"
	case StatusProcessing:
		return "\x1b[33m\x1b[1mProcessing\x1b[0m"
	case StatusError:
		return "\x1b[31m\x1b[1mError\x1b[0m"
	default:
		return status.String()
	}
}

func formatHeartbeatAge(lastHeartbeat time.Time) string {
	age := time.Since(lastHeartbeat)
	if age < time.Second {
		return "<1s ago"
	}
	if age < time.Minute {
		return fmt.Sprintf("%ds ago", int(age.Seconds()))
	}
	return fmt.Sprintf("%dm ago", int(age.Minutes()))
}

// visibleLength returns the visible length of a string, excluding ANSI codes.
func (m *RootModel) visibleLength(s string) int {
	length := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

// startPTY starts Claude Code in a PTY.
func (m *RootModel) startPTY() error {
	// Use --dangerously-skip-permissions to bypass interactive permission dialogs
	// This is appropriate for automated job execution in the harness
	args := []string{"--dangerously-skip-permissions"}
	if m.mcpConfigPath != "" {
		args = append(args, "--mcp-config", m.mcpConfigPath)
	}
	cmd := exec.CommandContext(m.ctx, "claude", args...)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"FORCE_COLOR=1",
		"MUSH_SIGNAL_DIR="+m.signalDir,
	)

	// Calculate PTY size (full height minus status bar)
	ptyHeight := m.ptyRowsForHeight(m.height)

	startWithSize := m.startPTYWithSize
	if startWithSize == nil {
		startWithSize = pty.StartWithSize
	}

	ptmx, err := startWithSize(cmd, &pty.Winsize{
		Rows: uint16(ptyHeight), //nolint:gosec // G115: terminal dimensions bounded by OS
		Cols: uint16(m.width),   //nolint:gosec // G115: terminal dimensions bounded by OS
	})
	if err != nil {
		return annotateStartPTYError(err, cmd.Path)
	}

	m.ptyMu.Lock()
	m.ptmx = ptmx
	m.cmd = cmd
	m.cmdPGID = 0
	if cmd.Process != nil && cmd.Process.Pid > 0 {
		if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil {
			m.cmdPGID = pgid
		}
	}
	m.ptyMu.Unlock()

	// Drain stale handles before delivering the new one, so the reader
	// always picks up the most recent PTY.
	for len(m.ptyReady) > 0 {
		<-m.ptyReady
	}
	m.ptyReady <- ptmx

	return nil
}

// closePTY closes the PTY.
func (m *RootModel) closePTY() {
	m.ptyMu.Lock()
	ptmx := m.ptmx
	cmd := m.cmd
	pgid := m.cmdPGID
	m.ptmx = nil
	m.cmd = nil
	m.cmdPGID = 0
	m.ptyMu.Unlock()

	if ptmx != nil {
		_ = ptmx.Close()
	}
	if cmd == nil || cmd.Process == nil {
		return
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	m.sendSignal(cmd.Process.Pid, pgid, syscall.SIGTERM)

	shutdownDeadline := m.ptyShutdownDeadline
	if shutdownDeadline <= 0 {
		shutdownDeadline = defaultPTYShutdownDeadline
	}

	select {
	case <-waitCh:
		return
	case <-time.After(shutdownDeadline):
		m.sendSignal(cmd.Process.Pid, pgid, syscall.SIGKILL)
		select {
		case <-waitCh:
		case <-time.After(shutdownDeadline):
		}
	}
}

func (m *RootModel) sendSignal(pid, pgid int, sig syscall.Signal) {
	kill := m.killProcess
	if kill == nil {
		kill = syscall.Kill
	}

	if pgid > 0 {
		if err := kill(-pgid, sig); err == nil || errors.Is(err, syscall.ESRCH) {
			return
		}
	}

	if pid <= 0 {
		return
	}
	_ = kill(pid, sig)
}

func annotateStartPTYError(err error, binaryPath string) error {
	if !errors.Is(err, syscall.EPERM) {
		return err
	}
	return fmt.Errorf(
		"%w (EPERM during PTY start for %q; likely session/exec policy issue. Check executable permissions, filesystem noexec, and macOS quarantine attributes)",
		err,
		binaryPath,
	)
}

func (m *RootModel) activePTY() *os.File {
	m.ptyMu.Lock()
	defer m.ptyMu.Unlock()
	return m.ptmx
}

// copyPTYOutput copies PTY output to stdout and detects prompt/completion.
// When readPTYOutput returns (PTY closed or error), the loop waits for a new
// PTY handle on m.ptyReady — this supports restart without signaling done.
// Job completion is detected via the Stop hook signal file, not PTY exit.
func (m *RootModel) copyPTYOutput() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		case ptmx := <-m.ptyReady:
			if ptmx == nil {
				continue
			}
			m.readPTYOutput(ptmx)
		}
	}
}

func (m *RootModel) readPTYOutput(ptmx *os.File) {
	buf := make([]byte, 4096)
	// Ring buffer to detect prompt across chunk boundaries
	promptRing := make([]byte, len(PromptDetectionBytes))
	promptRingIdx := 0
	// Buffer for bypass dialog detection
	var dialogBuf bytes.Buffer

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		default:
		}

		n, err := ptmx.Read(buf)
		if err != nil {
			return
		}

		if n <= 0 {
			continue
		}

		// Write to terminal
		m.termWrite(buf[:n])
		m.appendTranscript("pty", buf[:n])

		// Detect bypass dialog and auto-accept
		m.jobMu.Lock()
		if !m.bypassAccepted {
			dialogBuf.Write(buf[:n])
			// Look for "Esc to cancel" which appears at the end of the dialog
			if bytes.Contains(dialogBuf.Bytes(), []byte("Esc to cancel")) {
				m.bypassAccepted = true
				m.jobMu.Unlock()
				dialogBuf.Reset()
				// Wait a moment for the TUI to be ready, then send input
				go func() {
					time.Sleep(300 * time.Millisecond)
					active := m.activePTY()
					if active != nil {
						// Down arrow to select "Yes, I accept"
						_, _ = active.WriteString("\x1b[B")
						time.Sleep(100 * time.Millisecond)
						// Enter to confirm
						_, _ = active.WriteString("\r")
					}
				}()
			} else {
				m.jobMu.Unlock()
			}
		} else {
			m.jobMu.Unlock()
		}

		// Capture output if we're processing a job
		m.jobMu.Lock()
		if m.capturing {
			m.outputBuffer.Write(buf[:n])
		}
		// Reset prompt confirmation since new output arrived
		m.promptConfirmed = false
		m.jobMu.Unlock()

		// Detect prompt pattern "❯ " to know Claude might be ready
		for i := 0; i < n; i++ {
			promptRing[promptRingIdx] = buf[i]
			promptRingIdx = (promptRingIdx + 1) % len(PromptDetectionBytes)

			// Check if we have a match (need to check rotated)
			if m.checkPromptMatch(promptRing, promptRingIdx) {
				m.onPromptPatternSeen()
			}
		}
	}
}

// onPromptPatternSeen is called when we see "❯ " in the output.
// We use debouncing to avoid false positives from permission menus.
func (m *RootModel) onPromptPatternSeen() {
	m.jobMu.Lock()
	m.lastPromptSeen = time.Now()
	m.jobMu.Unlock()

	// Start a debounce goroutine
	go func() {
		time.Sleep(PromptDebounceTime)

		m.jobMu.Lock()
		// Check if this is still the most recent prompt sighting
		// and no new output has come (promptConfirmed would be false if output came)
		timeSincePrompt := time.Since(m.lastPromptSeen)
		if timeSincePrompt >= PromptDebounceTime-10*time.Millisecond && !m.promptConfirmed {
			m.promptConfirmed = true
			m.jobMu.Unlock()
			m.onPromptConfirmed()
		} else {
			m.jobMu.Unlock()
		}
	}()
}

// checkPromptMatch checks if the ring buffer contains the prompt bytes.
func (m *RootModel) checkPromptMatch(ring []byte, idx int) bool {
	for i := 0; i < len(PromptDetectionBytes); i++ {
		ringIdx := (idx + i) % len(ring)
		if ring[ringIdx] != PromptDetectionBytes[i] {
			return false
		}
	}
	return true
}

// onPromptConfirmed is called after debounce confirms Claude is at input prompt.
// This is only used for detecting initial readiness, not job completion.
// Job completion is detected via the Stop hook signal file.
func (m *RootModel) onPromptConfirmed() {
	m.jobMu.Lock()
	m.readyForJob = true
	m.jobMu.Unlock()

	// Signal prompt detected (non-blocking)
	select {
	case m.promptDetected <- struct{}{}:
	default:
	}
}

// copyInput copies stdin to PTY with quit key handling.
// Uses Ctrl+Q (0x11) as quit key to avoid escape sequence ambiguity.
func (m *RootModel) copyInput() {
	buf := make([]byte, 256)
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}

		// Check for local control keys first.
		for i := 0; i < n; i++ {
			if m.isCopyMode() && m.popCopyEscPending() {
				if buf[i] == '[' || buf[i] == 'O' {
					// This byte continues an escape sequence (e.g. arrow keys).
					// Stay in copy mode and swallow this input.
					buf[i] = 0
					continue
				}
				// Previous ESC was standalone; exit copy mode and process
				// this byte as normal input.
				m.setCopyMode(false)
			}

			if buf[i] == ctrlQ { // Ctrl+Q
				m.signalDone()
				return
			}
			if buf[i] == ctrlC { // Ctrl+C
				if m.handleCtrlC() {
					return
				}
				// We already handled forwarding locally; do not write this byte twice.
				buf[i] = 0
				continue
			}
			if buf[i] == ctrlS { // Ctrl+S toggles copy mode
				m.setCopyMode(!m.isCopyMode())
				// Drop the key from forwarded data.
				buf[i] = 0
				continue
			}
			if m.isCopyMode() {
				// Esc exits copy mode unless it prefixes a terminal escape sequence
				// like arrows/function keys.
				if buf[i] == esc {
					if i+1 < n && (buf[i+1] == '[' || buf[i+1] == 'O') {
						buf[i] = 0
						continue
					}
					m.setCopyEscPending(true)
				}
				buf[i] = 0
			}
		}

		// Forward filtered bytes to PTY.
		out := make([]byte, 0, n)
		for i := 0; i < n; i++ {
			if buf[i] != 0 {
				out = append(out, buf[i])
			}
		}
		if len(out) == 0 {
			continue
		}
		if ptmx := m.activePTY(); ptmx != nil {
			_, _ = ptmx.Write(out)
		}
	}
}

func (m *RootModel) handleCtrlC() bool {
	// When Claude is not actively running a job in PTY, Ctrl+C exits immediately.
	if !m.hasActiveClaudeJob() || m.activePTY() == nil {
		m.signalDone()
		return true
	}

	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()

	window := m.ctrlCExitWindow
	if window <= 0 {
		window = defaultCtrlCExitWindow
	}

	m.inputMu.Lock()
	last := m.lastCtrlCAt
	secondPress := !last.IsZero() && now.Sub(last) <= window
	if secondPress {
		m.lastCtrlCAt = time.Time{}
	} else {
		m.lastCtrlCAt = now
	}
	m.inputMu.Unlock()

	if secondPress {
		m.infof("Second Ctrl+C received: exiting watch mode.")
		m.signalDone()
		return true
	}

	if ptmx := m.activePTY(); ptmx != nil {
		_, _ = ptmx.Write([]byte{ctrlC})
	}
	m.infof("Interrupt sent to Claude. Press Ctrl+C again within %s to exit watch mode.", window.Round(time.Second))
	return false
}

func (m *RootModel) hasActiveClaudeJob() bool {
	m.jobMu.Lock()
	defer m.jobMu.Unlock()
	if m.currentJob == nil {
		return false
	}
	return m.currentJob.GetAgentType() == "claude"
}

// updateStatusLoop periodically updates the status bar.
func (m *RootModel) updateStatusLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		case <-ticker.C:
			m.drawStatusBar()
		}
	}
}

// jobManagerLoop manages the job queue and lifecycle by polling the API.
func (m *RootModel) jobManagerLoop() {
	// Wait for Claude to be ready (only when a Claude PTY is running).
	if m.activePTY() != nil {
		m.waitForClaude()
	} else {
		m.statusMu.Lock()
		m.status = StatusConnected
		m.statusMu.Unlock()
	}

	pollInterval := m.cfg.PollInterval()
	if pollInterval <= 0 {
		pollInterval = config.DefaultPollInterval
	}

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		default:
		}

		if err := m.maybeRestartClaude(); err != nil {
			m.setLastError(fmt.Sprintf("Claude restart failed: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}

		// Poll for a job
		job, err := m.client.ClaimJob(m.ctx, m.habitatID, m.queueID, pollInterval)
		if err != nil {
			if m.ctx.Err() != nil {
				return // Context canceled
			}
			m.setLastError(fmt.Sprintf("Claim failed: %v", err))
			time.Sleep(5 * time.Second) // Backoff on error
			continue
		}

		if job == nil {
			continue // No job, poll again
		}

		// Server contract uses execution.agentType; we map that to harness selection locally.
		agentType := job.GetAgentType()
		if !m.isHarnessSupported(agentType) {
			errMsg := fmt.Sprintf("Unsupported harness type: %s", agentType)
			m.setLastError(errMsg)
			m.releaseJob(job)
			continue
		}

		// Process the job
		m.processJob(job)
	}
}

// waitForClaude waits until the Claude PTY is ready for input.
func (m *RootModel) waitForClaude() {
	m.drainPromptDetected()

	m.statusMu.Lock()
	m.status = StatusConnected
	m.statusMu.Unlock()

	select {
	case <-m.ctx.Done():
	case <-m.done:
	case <-m.promptDetected:
		// Claude is ready (prompt detected)
	case <-time.After(15 * time.Second):
		// Fallback timeout
		m.jobMu.Lock()
		if m.bypassAccepted {
			m.jobMu.Unlock()
			time.Sleep(2 * time.Second)
		} else {
			m.jobMu.Unlock()
		}
	}
}

func (m *RootModel) drainPromptDetected() {
	for {
		select {
		case <-m.promptDetected:
			// Drain stale prompt events.
		default:
			return
		}
	}
}

// processJob handles the lifecycle of a single job.
func (m *RootModel) processJob(job *client.Job) {
	agentType := job.GetAgentType()

	m.jobMu.Lock()
	m.currentJob = job
	m.readyForJob = false
	if agentType == "claude" {
		m.capturing = true
		m.outputBuffer.Reset()
	} else {
		m.capturing = false
		m.outputBuffer.Reset()
	}
	m.jobMu.Unlock()

	// Update status bar
	m.statusMu.Lock()
	m.status = StatusProcessing
	m.statusMu.Unlock()
	m.drawStatusBar()

	// Start heartbeat for the job
	m.heartbeatCtx, m.heartbeatCancel = context.WithCancel(m.ctx)
	go m.heartbeatLoop(m.heartbeatCtx, job.ID)

	defer func() {
		m.heartbeatCancel()
		m.jobMu.Lock()
		m.currentJob = nil
		m.capturing = false
		m.jobMu.Unlock()
		m.inputMu.Lock()
		m.lastCtrlCAt = time.Time{}
		m.inputMu.Unlock()
		m.statusMu.Lock()
		m.status = StatusConnected
		m.statusMu.Unlock()
		_ = os.Remove(m.currentJobPath())
		_ = os.Remove(m.signalPath())
	}()

	if _, err := m.client.StartJob(m.ctx, job.ID); err != nil {
		m.setLastError(fmt.Sprintf("Start job failed: %v", err))
	}

	// Clear any prior signal file and record current job (Claude only).
	if agentType == "claude" && m.signalDir != "" {
		_ = os.Remove(m.signalPath())
		_ = os.WriteFile(m.currentJobPath(), []byte(job.ID), 0o600)
	}

	// Determine execution timeout: use job-specified value or fall back to default
	execTimeout := DefaultExecutionTimeout
	if job.Execution != nil && job.Execution.TimeoutMs > 0 {
		execTimeout = time.Duration(job.Execution.TimeoutMs) * time.Millisecond
	}

	// Execute by harness type. (The wire contract still calls this `agentType`.)
	switch agentType {
	case "claude":
		// Get the prompt for Claude
		prompt, err := m.getPromptFromJob(job)
		if err != nil {
			m.setLastError(fmt.Sprintf("Prompt error: %v", err))
			m.failJobNoRetry(job, "prompt_error", err.Error())
			return
		}

		// Inject the prompt into the PTY
		m.injectPrompt(prompt)

		startedAt := time.Now()

		// Wait for completion signal with timeout
		waitCtx, cancelWait := context.WithTimeout(m.ctx, execTimeout)
		defer cancelWait()

		output, execErr := m.waitForSignalFile(waitCtx)
		duration := time.Since(startedAt)

		if execErr != nil {
			reason := "execution_error"
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				reason = "timeout"
			}
			m.failJob(job, reason, execErr.Error())
			return
		}

		m.completeJob(job, map[string]any{
			"success":    true,
			"output":     output,
			"durationMs": int(duration / time.Millisecond),
		})

		// Send /clear to reset session
		m.sendClear()

		// Wait for prompt to reappear after /clear
		select {
		case <-m.ctx.Done():
		case <-m.done:
		case <-m.promptDetected:
		case <-time.After(10 * time.Second):
		}

		time.Sleep(1 * time.Second) // Settle time
		return

	case "bash":
		command, err := m.getBashCommandFromJob(job)
		if err != nil {
			m.setLastError(fmt.Sprintf("Command error: %v", err))
			m.failJobNoRetry(job, "command_error", err.Error())
			return
		}

		startedAt := time.Now()

		waitCtx, cancelWait := context.WithTimeout(m.ctx, execTimeout)
		defer cancelWait()

		outputData, execErr := m.executeBashJob(waitCtx, job, command)
		duration := time.Since(startedAt)

		if execErr != nil {
			reason := "bash_error"
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				reason = "timeout"
			}
			m.failJob(job, reason, execErr.Error())
			return
		}

		_ = duration // durationMs is set by executeBashJob on success.
		outputData["success"] = true

		m.completeJob(job, outputData)
		return

	default:
		m.setLastError(fmt.Sprintf("Unsupported harness type: %s", agentType))
		m.releaseJob(job)
		return
	}
}

// getPromptFromJob extracts the prompt from a job's data and execution config.
func (m *RootModel) getPromptFromJob(job *client.Job) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}

	if rendered := job.GetRenderedInstruction(); rendered != "" {
		return rendered, nil
	}

	// If the API returned an execution error, surface it
	if job.ExecutionError != "" {
		return "", fmt.Errorf("server execution error: %s", job.ExecutionError)
	}

	if job.InputData != nil {
		if instruction, ok := job.InputData["instruction"].(string); ok && instruction != "" {
			return instruction, nil
		}
		if title, ok := job.InputData["title"].(string); ok && title != "" {
			if desc, ok := job.InputData["description"].(string); ok && desc != "" {
				return title + "\n\n" + desc, nil
			}
			return title, nil
		}
		if prompt, ok := job.InputData["prompt"].(string); ok && prompt != "" {
			return prompt, nil
		}
	}

	return "", fmt.Errorf("no prompt found for job")
}

func (m *RootModel) getBashCommandFromJob(job *client.Job) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}

	// Prefer pre-rendered instruction from the server.
	if rendered := job.GetRenderedInstruction(); rendered != "" {
		return rendered, nil
	}

	// If the API returned an execution error, surface it
	if job.ExecutionError != "" {
		return "", fmt.Errorf("server execution error: %s", job.ExecutionError)
	}

	if job.InputData != nil {
		if cmd, ok := job.InputData["command"].(string); ok && cmd != "" {
			return cmd, nil
		}
		if script, ok := job.InputData["script"].(string); ok && script != "" {
			return script, nil
		}
	}

	return "", fmt.Errorf("no bash command found for job")
}

// heartbeatLoop sends periodic heartbeats for the current job.
func (m *RootModel) heartbeatLoop(ctx context.Context, jobID string) {
	interval := m.cfg.HeartbeatInterval()
	if interval <= 0 {
		interval = config.DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := m.client.HeartbeatJob(m.ctx, jobID)
			if err != nil {
				m.setLastError(fmt.Sprintf("Heartbeat failed: %v", err))
				continue
			}

			m.statusMu.Lock()
			m.lastHeartbeat = time.Now()
			m.statusMu.Unlock()
		}
	}
}

// waitForSignalFile polls for a completion signal file from the Stop hook.
// It returns the captured output string when the signal is detected, or a non-nil
// error if the context is done (including timeout/cancellation) or if the harness
// has been stopped.
func (m *RootModel) waitForSignalFile(ctx context.Context) (string, error) {
	ticker := time.NewTicker(SignalPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-m.done:
			return "", errors.New("harness stopped")
		case <-ticker.C:
			if _, err := os.Stat(m.signalPath()); err != nil {
				continue
			}

			_ = os.Remove(m.signalPath()) // Clean up signal

			m.jobMu.Lock()
			m.capturing = false
			output := ansi.Strip(m.outputBuffer.String())
			m.outputBuffer.Reset()
			m.jobMu.Unlock()
			return output, nil
		}
	}
}

// injectPrompt writes a prompt into the PTY in bulk chunks.
func (m *RootModel) injectPrompt(prompt string) {
	ptmx := m.activePTY()
	if ptmx == nil {
		return
	}

	// Write in chunks to respect PTY buffer limits
	data := []byte(prompt)
	for len(data) > 0 {
		chunk := data
		if len(chunk) > PTYWriteChunkSize {
			chunk = data[:PTYWriteChunkSize]
		}
		_, _ = ptmx.Write(chunk)
		data = data[len(chunk):]
		if len(data) > 0 {
			time.Sleep(PTYChunkDelay)
		}
	}

	// Allow the application to finish processing the pasted content
	time.Sleep(PTYPasteSettleDelay)
	_, _ = ptmx.WriteString("\r")
}

// sendClear sends the /clear command to reset Claude's session.
func (m *RootModel) sendClear() {
	ptmx := m.activePTY()
	if ptmx == nil {
		return
	}
	time.Sleep(500 * time.Millisecond)
	_, _ = ptmx.WriteString("/clear")
	time.Sleep(PTYPostWriteDelay)
	_, _ = ptmx.WriteString("\r")
}

// completeJob reports job completion to the API.
func (m *RootModel) completeJob(job *client.Job, outputData map[string]any) {
	err := m.client.CompleteJob(m.ctx, job.ID, outputData)
	if err != nil {
		m.setLastError(fmt.Sprintf("Complete failed: %v", err))
		// If completion fails, we should probably try to fail it as a fallback
		m.failJob(job, "completion_report_failed", err.Error())
		return
	}

	m.statusMu.Lock()
	m.completed++
	m.statusMu.Unlock()
}

func (m *RootModel) executeBashJob(ctx context.Context, job *client.Job, command string) (map[string]any, error) {
	// Verify bash is available
	if _, err := exec.LookPath("bash"); err != nil {
		return nil, fmt.Errorf("bash not found in PATH")
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	// Working directory
	if job.Execution != nil && job.Execution.WorkingDirectory != "" {
		cmd.Dir = job.Execution.WorkingDirectory
	}

	// Environment variables
	cmd.Env = os.Environ()
	if job.Execution != nil {
		for k, v := range job.Execution.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("MUSH_JOB_ID=%s", job.ID),
		fmt.Sprintf("MUSH_JOB_NAME=%s", job.GetDisplayName()),
		fmt.Sprintf("MUSH_JOB_QUEUE=%s", job.QueueID),
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	termStdout := &lockedWriter{mu: &m.termMu, w: os.Stdout}
	termStderr := &lockedWriter{mu: &m.termMu, w: os.Stderr}
	cmd.Stdout = io.MultiWriter(termStdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(termStderr, &stderrBuf)

	startedAt := time.Now()
	err := cmd.Run()
	duration := time.Since(startedAt)

	exitCode := 0
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			// Prefer context cancellation/timeout errors over synthetic exit codes.
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("bash execution timed out: %w", ctxErr)
			}
			return nil, fmt.Errorf("bash execution canceled: %w", ctxErr)
		}

		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("bash exited with code %d: %s", exitCode, msg)
	}

	return map[string]any{
		"output":     ansi.Strip(strings.TrimSpace(stdoutBuf.String())),
		"stdout":     stdoutBuf.String(),
		"stderr":     stderrBuf.String(),
		"exitCode":   exitCode,
		"durationMs": int(duration / time.Millisecond),
	}, nil
}

// releaseJob returns a job to the queue.
func (m *RootModel) releaseJob(job *client.Job) {
	if err := m.client.ReleaseJob(m.ctx, job.ID); err != nil {
		m.setLastError(fmt.Sprintf("Release failed: %v", err))
	}
}

// failJob reports job failure to the API (retryable).
func (m *RootModel) failJob(job *client.Job, reason, message string) {
	err := m.client.FailJob(m.ctx, job.ID, reason, message, true)
	if err != nil {
		m.setLastError(fmt.Sprintf("Fail report failed: %v", err))
	}

	m.statusMu.Lock()
	m.failed++
	m.statusMu.Unlock()
}

// failJobNoRetry reports a permanent job failure (no retry).
func (m *RootModel) failJobNoRetry(job *client.Job, reason, message string) {
	err := m.client.FailJob(m.ctx, job.ID, reason, message, false)
	if err != nil {
		m.setLastError(fmt.Sprintf("Fail report failed: %v", err))
	}

	m.statusMu.Lock()
	m.failed++
	m.statusMu.Unlock()
}

func (m *RootModel) signalPath() string {
	if m.signalDir == "" {
		return ""
	}
	return filepath.Join(m.signalDir, SignalFileName)
}

func (m *RootModel) currentJobPath() string {
	if m.signalDir == "" {
		return ""
	}
	return filepath.Join(m.signalDir, "current-job")
}

func (m *RootModel) currentJobID() string {
	m.jobMu.Lock()
	defer m.jobMu.Unlock()
	if m.currentJob == nil {
		return ""
	}
	return m.currentJob.ID
}

// setLastError records an error to be displayed in the status bar.
func (m *RootModel) setLastError(msg string) {
	m.statusMu.Lock()
	m.lastError = msg
	m.lastErrorTime = time.Now()
	m.statusMu.Unlock()
}

func (m *RootModel) runnerConfigRefreshLoop() {
	interval := m.refreshInterval
	if interval <= 0 {
		interval = normalizeRefreshInterval(0)
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.done:
			return
		case <-timer.C:
			cfg, err := m.client.GetRunnerConfig(m.ctx)
			if err != nil {
				m.setLastError(fmt.Sprintf("Runner config refresh failed: %v", err))
				timer.Reset(interval)
				continue
			}

			specs := buildMCPProviderSpecs(cfg, time.Now())
			sig, sigErr := mcpSignature(specs)
			if sigErr != nil {
				m.setLastError(fmt.Sprintf("Runner config signature failed: %v", sigErr))
				timer.Reset(interval)
				continue
			}

			m.refreshMu.Lock()
			interval = normalizeRefreshInterval(cfg.RefreshAfterSeconds)
			m.refreshInterval = interval
			if sig != m.mcpConfigSig {
				m.pendingRunnerConfig = cfg
				m.pendingRunnerConfigSig = sig
				m.claudeRestartNeeded = true
			}
			m.refreshMu.Unlock()
			timer.Reset(interval)
		}
	}
}

func (m *RootModel) maybeRestartClaude() error {
	if !m.isHarnessSupported("claude") {
		return nil
	}
	if m.currentJobID() != "" {
		return nil
	}

	m.refreshMu.Lock()
	needsRestart := m.claudeRestartNeeded
	nextCfg := m.pendingRunnerConfig
	oldNames := append([]string(nil), m.loadedMCPNames...)
	m.refreshMu.Unlock()

	if !needsRestart || nextCfg == nil {
		return nil
	}

	if err := m.applyRunnerConfigForClaude(nextCfg); err != nil {
		return err
	}

	m.closePTY()
	if err := m.startPTY(); err != nil {
		return err
	}
	m.waitForClaude()

	m.refreshMu.Lock()
	m.pendingRunnerConfig = nil
	m.pendingRunnerConfigSig = ""
	m.claudeRestartNeeded = false
	newNames := append([]string(nil), m.loadedMCPNames...)
	m.refreshMu.Unlock()

	if !sameStringSlice(oldNames, newNames) {
		m.infof("MCP servers reloaded: %s", summarizeMCPServers(newNames))
	}

	return nil
}

func (m *RootModel) applyRunnerConfigForClaude(cfg *client.RunnerConfigResponse) error {
	now := time.Now()
	path, sig, cleanup, err := createClaudeMCPConfigFile(cfg, now)
	if err != nil {
		return err
	}
	names := loadedMCPProviderNames(cfg, now)

	// Swap config file atomically after successful generation.
	m.refreshMu.Lock()
	oldCleanup := m.mcpConfigRemove
	m.mcpConfigPath = path
	m.mcpConfigSig = sig
	m.mcpConfigRemove = cleanup
	m.loadedMCPNames = names
	m.refreshMu.Unlock()

	m.runnerConfig = cfg

	if oldCleanup != nil {
		if err := oldCleanup(); err != nil {
			m.setLastError(fmt.Sprintf("MCP config cleanup: %v", err))
		}
	}

	return nil
}

func (m *RootModel) cleanupMCPConfigFile() {
	if m.mcpConfigRemove == nil {
		return
	}
	if err := m.mcpConfigRemove(); err != nil {
		m.setLastError(fmt.Sprintf("MCP config cleanup: %v", err))
	}
	m.mcpConfigRemove = nil
	m.mcpConfigPath = ""
}

func (m *RootModel) infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	// Use \r\n because the terminal is in raw mode; writing through termWrite
	// ensures the output lands inside the scroll region alongside PTY output.
	m.termWrite([]byte(msg + "\r\n"))
}

func summarizeMCPServers(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := append([]string(nil), a...)
	bCopy := append([]string(nil), b...)
	slices.Sort(aCopy)
	slices.Sort(bCopy)
	return slices.Equal(aCopy, bCopy)
}

// restore restores the terminal to its original state.
func (m *RootModel) restore() {
	// Reset scroll region
	m.termWriteString(escResetScroll)

	// Show cursor
	m.termWriteString(escShowCursor)

	// Reset colors
	m.termWriteString(escReset)

	// Move cursor to bottom
	m.termPrintf(escMoveTo, m.height, 1)
	m.termWriteString("\n")

	// Restore terminal state
	if m.oldState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), m.oldState)
	}
}
