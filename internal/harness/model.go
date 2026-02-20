//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"golang.org/x/term"

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

// ANSI escape sequences for terminal control.
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

// StatusBarHeight is the number of lines reserved for the status bar.
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

	// Executor registry: harness type → executor instance.
	// Populated once during Run() setup and never modified after.
	// Safe for concurrent read access without a mutex.
	executors map[string]Executor

	// setPTYSize is injectable for tests; defaults to pty.Setsize.
	setPTYSize func(*os.File, *pty.Winsize) error

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
	runnerConfig       *client.RunnerConfigResponse
	transcriptEnabled  bool
	transcriptDir      string
	transcriptLines    int

	refreshMu       sync.Mutex
	refreshInterval time.Duration

	// Original terminal state for restoration
	oldState *term.State

	// Control channels
	done      chan struct{}
	closeOnce sync.Once // Ensures done channel is closed only once

	// Job lifecycle management
	jobMu           sync.Mutex
	currentJob      *client.Job // Currently executing job
	lastError       string      // Last error message
	lastErrorTime   time.Time   // Time of the last error
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc

	// Load mode fields
	bundleLoadMode bool
	bundleName     string
	bundleVer      string
	bundleDir      string

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

	written, err := lw.w.Write(p)
	if err != nil {
		return written, fmt.Errorf("write to locked writer: %w", err)
	}

	return written, nil
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
		executors:           make(map[string]Executor),
		setPTYSize:          pty.Setsize,
		transcriptEnabled:   cfg.TranscriptEnabled,
		transcriptDir:       cfg.TranscriptDir,
		transcriptLines:     cfg.TranscriptLines,
		bundleLoadMode:      cfg.BundleLoadMode,
		bundleName:          cfg.BundleName,
		bundleVer:           cfg.BundleVer,
		bundleDir:           cfg.BundleDir,
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
	if m.client == nil && !m.bundleLoadMode {
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

	// Create per-run signal directory (used by Claude executor).
	if m.isHarnessSupported("claude") {
		signalDir, mkErr := os.MkdirTemp("", "mush-signals-")
		if mkErr != nil {
			return fmt.Errorf("failed to create signal directory: %w", mkErr)
		}

		m.signalDir = signalDir

		defer func() {
			_ = os.RemoveAll(signalDir)
		}()
	}

	// Create executors from registry.
	ptyRows := m.ptyRowsForHeight(m.height)
	termWriter := &lockedWriter{mu: &m.termMu, w: os.Stdout}

	for _, harnessType := range m.supportedHarnesses {
		info, ok := Lookup(harnessType)
		if !ok {
			continue
		}

		executor := info.New()

		setupOpts := SetupOptions{
			TermWriter:     termWriter,
			TermWidth:      m.width,
			TermHeight:     ptyRows,
			SignalDir:      m.signalDir,
			RunnerConfig:   m.runnerConfig,
			BundleDir:      m.bundleDir,
			BundleLoadMode: m.bundleLoadMode,
			OnOutput: func(p []byte) {
				m.appendTranscript("pty", p)
			},
			OnExit: m.signalDone,
		}

		if err := executor.Setup(m.ctx, &setupOpts); err != nil {
			return fmt.Errorf("failed to setup %s executor: %w", harnessType, err)
		}

		m.executors[harnessType] = executor
	}

	defer func() {
		for _, executor := range m.executors {
			executor.Teardown()
		}
	}()

	if m.bundleLoadMode {
		return m.runBundleLoadMode()
	}

	return m.runLinkMode()
}

// runLinkMode runs the standard job-polling link mode.
func (m *RootModel) runLinkMode() error {
	// Register link with the platform.
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

	// Start goroutines.
	var wg sync.WaitGroup

	// Terminal resize watcher (SIGWINCH + periodic reconciliation).
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.resizeLoop()
	}()

	// Stdin → PTY (with quit key handling).
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.copyInput()
	}()

	// Status bar updater.
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.updateStatusLoop()
	}()

	// Job manager (polls for and processes jobs).
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.jobManagerLoop()
	}()

	// Runner config refresh loop for MCP credential rotation.
	if _, ok := m.executors["claude"]; ok {
		wg.Add(1)

		go func() {
			defer wg.Done()

			m.runnerConfigRefreshLoop()
		}()
	}

	// Ensure external context cancellation can always unblock Run().
	go func() {
		select {
		case <-m.ctx.Done():
			m.signalDone()
		case <-m.done:
		}
	}()

	// Wait for done signal.
	<-m.done

	m.cancel()
	wg.Wait()

	return nil
}

// runBundleLoadMode runs a single interactive session with bundle assets.
func (m *RootModel) runBundleLoadMode() error {
	m.statusMu.Lock()
	m.status = StatusConnected
	m.statusMu.Unlock()

	// Start goroutines.
	var wg sync.WaitGroup

	// Terminal resize watcher.
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.resizeLoop()
	}()

	// Stdin → active executor.
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.copyInput()
	}()

	// Status bar updater.
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.updateStatusLoop()
	}()

	// Ensure external context cancellation can always unblock Run().
	go func() {
		select {
		case <-m.ctx.Done():
			m.signalDone()
		case <-m.done:
		}
	}()

	// Wait for done signal (user presses Ctrl+Q or executor process exits).
	<-m.done

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
		return 0, 0, fmt.Errorf("get terminal size: %w", err)
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

	rows := m.ptyRowsForHeight(height)

	// Resize all Resizable executors.
	for _, executor := range m.executors {
		if r, ok := executor.(Resizable); ok {
			r.Resize(rows, width)
		}
	}

	m.drawStatusBar()
}

// copyInput copies stdin to active executor with quit key handling.
// Uses Ctrl+Q (0x11) as quit key to avoid escape sequence ambiguity.
func (m *RootModel) copyInput() {
	buf := make([]byte, 256)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		bytesRead, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}

		// Check for local control keys first.
		for i := 0; i < bytesRead; i++ {
			if m.isCopyMode() && m.popCopyEscPending() {
				if buf[i] == '[' || buf[i] == 'O' {
					buf[i] = 0
					continue
				}

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

				buf[i] = 0

				continue
			}

			if buf[i] == ctrlS { // Ctrl+S toggles copy mode
				m.setCopyMode(!m.isCopyMode())

				buf[i] = 0

				continue
			}

			if m.isCopyMode() {
				if buf[i] == esc {
					if i+1 < bytesRead && (buf[i+1] == '[' || buf[i+1] == 'O') {
						buf[i] = 0
						continue
					}

					m.setCopyEscPending(true)
				}

				buf[i] = 0
			}
		}

		// Forward filtered bytes to active InputReceiver executor.
		out := make([]byte, 0, bytesRead)
		for i := 0; i < bytesRead; i++ {
			if buf[i] != 0 {
				out = append(out, buf[i])
			}
		}

		if len(out) == 0 {
			continue
		}

		// Write to the first InputReceiver executor.
		for _, harnessType := range m.supportedHarnesses {
			if executor, ok := m.executors[harnessType]; ok {
				if ir, ok := executor.(InputReceiver); ok {
					_, _ = ir.WriteInput(out)

					break
				}
			}
		}
	}
}

func (m *RootModel) handleCtrlC() bool {
	// When not actively running a Claude job, Ctrl+C exits immediately.
	if !m.hasActiveClaudeJob() {
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

	// Forward Ctrl+C to the Claude executor.
	if executor, ok := m.executors["claude"]; ok {
		if ir, ok := executor.(InputReceiver); ok {
			_, _ = ir.WriteInput([]byte{ctrlC})
		}
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

	return m.currentJob.GetHarnessType() == "claude"
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
	// Wait for Claude to be ready if it's a supported harness.
	m.statusMu.Lock()
	m.status = StatusConnected
	m.statusMu.Unlock()

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

		// Check if any Refreshable executors need restart.
		if err := m.maybeRefreshExecutors(); err != nil {
			m.setLastError(fmt.Sprintf("Executor refresh failed: %v", err))
			time.Sleep(2 * time.Second)

			continue
		}

		// Poll for a job.
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

		// Map execution.harnessType to local harness selection.
		harnessType := job.GetHarnessType()
		if !m.isHarnessSupported(harnessType) {
			errMsg := fmt.Sprintf("Unsupported harness type: %s", harnessType)
			m.setLastError(errMsg)
			m.releaseJob(job)

			continue
		}

		// Process the job.
		m.processJob(job)
	}
}

// processJob handles the lifecycle of a single job using the executor.
func (m *RootModel) processJob(job *client.Job) {
	harnessType := job.GetHarnessType()

	executor, ok := m.executors[harnessType]
	if !ok {
		m.setLastError(fmt.Sprintf("No executor for harness type: %s", harnessType))
		m.releaseJob(job)

		return
	}

	m.jobMu.Lock()
	m.currentJob = job
	m.jobMu.Unlock()

	// Update status bar
	m.statusMu.Lock()
	m.status = StatusProcessing
	m.statusMu.Unlock()
	m.drawStatusBar()

	// Start heartbeat for the job.
	m.heartbeatCtx, m.heartbeatCancel = context.WithCancel(m.ctx)
	go m.heartbeatLoop(m.heartbeatCtx, job.ID)

	defer func() {
		m.heartbeatCancel()
		m.jobMu.Lock()
		m.currentJob = nil
		m.jobMu.Unlock()
		m.inputMu.Lock()
		m.lastCtrlCAt = time.Time{}
		m.inputMu.Unlock()
		m.statusMu.Lock()
		m.status = StatusConnected
		m.statusMu.Unlock()
	}()

	if _, err := m.client.StartJob(m.ctx, job.ID); err != nil {
		m.setLastError(fmt.Sprintf("Start job failed: %v", err))
	}

	// Determine execution timeout.
	execTimeout := DefaultExecutionTimeout
	if job.Execution != nil && job.Execution.TimeoutMs > 0 {
		execTimeout = time.Duration(job.Execution.TimeoutMs) * time.Millisecond
	}

	execCtx, cancelExec := context.WithTimeout(m.ctx, execTimeout)
	defer cancelExec()

	// Execute the job via the executor.
	result, execErr := executor.Execute(execCtx, job)
	if execErr != nil {
		reason := "execution_error"
		msg := execErr.Error()
		retry := true

		var ee *ExecError
		if errors.As(execErr, &ee) {
			reason = ee.Reason
			msg = ee.Message
			retry = ee.Retry
		}

		if retry {
			m.failJob(job, reason, msg)
		} else {
			m.failJobNoRetry(job, reason, msg)
		}

		return
	}

	m.completeJob(job, result.OutputData)

	// Reset the executor for the next job.
	if err := executor.Reset(m.ctx); err != nil {
		m.setLastError(fmt.Sprintf("Executor reset failed: %v", err))
	}
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

// completeJob reports job completion to the API.
func (m *RootModel) completeJob(job *client.Job, outputData map[string]any) {
	err := m.client.CompleteJob(m.ctx, job.ID, outputData)
	if err != nil {
		m.setLastError(fmt.Sprintf("Complete failed: %v", err))
		m.failJob(job, "completion_report_failed", err.Error())

		return
	}

	m.statusMu.Lock()
	m.completed++
	m.statusMu.Unlock()
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

			m.refreshMu.Lock()
			interval = normalizeRefreshInterval(cfg.RefreshAfterSeconds)
			m.refreshInterval = interval

			// Check all refreshable executors.
			for _, executor := range m.executors {
				if r, ok := executor.(Refreshable); ok {
					if r.NeedsRefresh(cfg) {
						m.runnerConfig = cfg
					}
				}
			}

			m.refreshMu.Unlock()
			timer.Reset(interval)
		}
	}
}

func (m *RootModel) maybeRefreshExecutors() error {
	if m.currentJobID() != "" {
		return nil
	}

	m.refreshMu.Lock()
	cfg := m.runnerConfig
	m.refreshMu.Unlock()

	for harnessName, executor := range m.executors {
		r, ok := executor.(Refreshable)
		if !ok {
			continue
		}

		if !r.NeedsRefresh(cfg) {
			continue
		}

		if err := r.ApplyRefresh(m.ctx, cfg); err != nil {
			return fmt.Errorf("apply refresh for %s: %w", harnessName, err)
		}
	}

	return nil
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

func sameStringSlice(expected, compared []string) bool {
	if len(expected) != len(compared) {
		return false
	}

	aCopy := make([]string, len(expected))
	copy(aCopy, expected)

	bCopy := make([]string, len(compared))
	copy(bCopy, compared)

	sort.Strings(aCopy)
	sort.Strings(bCopy)

	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}

	return true
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
