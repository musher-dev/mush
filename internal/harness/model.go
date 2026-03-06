//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
	harnessstate "github.com/musher-dev/mush/internal/harness/state"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
	statusui "github.com/musher-dev/mush/internal/harness/ui/status"
	"github.com/musher-dev/mush/internal/transcript"
	"github.com/musher-dev/mush/internal/worker"
)

// ResizePollInterval is the fallback interval to reconcile terminal size in
// environments that don't reliably forward SIGWINCH.
const ResizePollInterval = 250 * time.Millisecond

// DefaultExecutionTimeout is the fallback when no execution timeout is set on the job.
const DefaultExecutionTimeout = 10 * time.Minute

const (
	ctrlC = 0x03
	ctrlQ = 0x11
	ctrlS = 0x13
	esc   = 0x1b
)

const (
	defaultCtrlCExitWindow     = 2 * time.Second
	defaultPTYShutdownDeadline = 3 * time.Second
)

// RootModel manages the harness state with scroll region approach.
// It composes TerminalController and JobLoop as focused sub-components.
type RootModel struct {
	ctx    context.Context
	cancel context.CancelFunc

	term *TerminalController
	jobs *JobLoop

	// Executor registry: harness type → executor instance.
	// Populated once during Run() setup and never modified after.
	// Shared by reference with term and jobs.
	executors map[string]harnesstype.Executor

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

	// Immutable configuration.
	cfg                *config.Config
	supportedHarnesses []string
	habitatID          string
	queueID            string
	transcriptEnabled  bool
	transcriptDir      string
	transcriptLines    int

	// Bundle load mode (immutable).
	bundleLoadMode bool
	bundleName     string
	bundleVer      string
	bundleDir      string
	bundleSummary  BundleSummary

	// Control channels.
	done      chan struct{}
	closeOnce sync.Once

	// Time and lifecycle behavior knobs (defaulted in constructor; injectable in tests).
	now                 func() time.Time
	ctrlCExitWindow     time.Duration
	ptyShutdownDeadline time.Duration
	killProcess         func(int, syscall.Signal) error
}

// NewRootModel creates a new root model with the given configuration.
func NewRootModel(ctx context.Context, cfg *Config) *RootModel {
	ctx, cancel := context.WithCancel(ctx)

	initialStatus := StatusConnecting
	if cfg.BundleLoadMode {
		initialStatus = StatusStarting
	}

	executors := make(map[string]harnesstype.Executor)
	loadedCfg := config.Load()

	model := &RootModel{
		ctx:    ctx,
		cancel: cancel,
		term: &TerminalController{
			executors:          executors,
			supportedHarnesses: cfg.SupportedHarnesses,
			forceSidebar:       cfg.ForceSidebar,
		},
		jobs: &JobLoop{
			client:             cfg.Client,
			cfg:                loadedCfg,
			habitatID:          cfg.HabitatID,
			queueID:            cfg.QueueID,
			instanceID:         cfg.InstanceID,
			executors:          executors,
			supportedHarnesses: cfg.SupportedHarnesses,
			status:             initialStatus,
			lastHeartbeat:      time.Now(),
			runnerConfig:       cfg.RunnerConfig,
			refreshInterval:    normalizeRefreshInterval(0),
		},
		executors:           executors,
		cfg:                 loadedCfg,
		supportedHarnesses:  cfg.SupportedHarnesses,
		habitatID:           cfg.HabitatID,
		queueID:             cfg.QueueID,
		done:                make(chan struct{}),
		setPTYSize:          pty.Setsize,
		transcriptEnabled:   cfg.TranscriptEnabled,
		transcriptDir:       cfg.TranscriptDir,
		transcriptLines:     cfg.TranscriptLines,
		bundleLoadMode:      cfg.BundleLoadMode,
		bundleName:          cfg.BundleName,
		bundleVer:           cfg.BundleVer,
		bundleDir:           cfg.BundleDir,
		bundleSummary:       cfg.BundleSummary,
		now:                 time.Now,
		ctrlCExitWindow:     defaultCtrlCExitWindow,
		ptyShutdownDeadline: defaultPTYShutdownDeadline,
		killProcess:         syscall.Kill,
	}

	// Wire callbacks.
	model.term.drawStatusBar = model.drawStatusBar
	model.term.setLastError = model.setLastError
	model.term.ptyActive = model.isPTYActive
	model.jobs.drawStatusBar = model.drawStatusBar
	model.jobs.infof = model.infof
	model.jobs.signalDone = model.signalDone
	model.jobs.now = model.now

	return model
}

// signalDone safely closes the done channel exactly once.
func (m *RootModel) signalDone() {
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

// Run starts the harness with scroll region approach.
func (m *RootModel) Run() error {
	if m.jobs.client == nil && !m.bundleLoadMode {
		return fmt.Errorf("missing client in harness config")
	}

	// Get terminal size
	width, height, err := m.term.readTerminalSize()
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	m.term.width = width
	m.term.height = height

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}

	m.term.oldState = oldState
	defer m.term.restore()

	if m.term.forceSidebar {
		m.term.lrMarginSupported.Store(true)
	} else {
		m.term.lrMarginSupported.Store(m.term.detectLRMarginSupport())
	}

	// Clear screen and set up scroll region
	m.term.setupScreen()

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
			historyLines = m.cfg.HistoryScrollbackLines()
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

	// Create per-run signal directory if any supported harness needs it.
	if needsSignalDir(m.supportedHarnesses) {
		signalDir, mkErr := os.MkdirTemp("", "mush-signals-")
		if mkErr != nil {
			return fmt.Errorf("failed to create signal directory: %w", mkErr)
		}

		m.jobs.signalDir = signalDir

		defer func() {
			_ = os.RemoveAll(signalDir)
		}()
	}

	// Create executors from registry.
	frame := layout.ComputeFrame(m.term.width, m.term.height, m.term.SidebarEnabled())
	ptyRows := layout.PtyRowsForFrame(frame)
	filter := &sidebarFilter{
		active: m.term.SidebarEnabled,
		paneDims: func() (int, int, int, int) {
			return int(m.term.filterContentTop.Load()),
				int(m.term.filterScrollBottom.Load()),
				int(m.term.filterPaneXStart.Load()),
				int(m.term.filterTermWidth.Load())
		},
	}
	termWriter := &lockedWriter{
		mu:      &m.term.mu,
		w:       os.Stdout,
		filter:  filter.rewrite,
		onWrite: m.term.inspectTerminalControlSequences,
	}

	for _, harnessType := range m.supportedHarnesses {
		info, ok := Lookup(harnessType)
		if !ok {
			continue
		}

		executor := info.New()

		setupOpts := harnesstype.SetupOptions{
			TermWriter:     termWriter,
			TermWidth:      frame.PaneWidth,
			TermHeight:     ptyRows,
			SignalDir:      m.jobs.signalDir,
			RunnerConfig:   m.jobs.runnerConfig,
			BundleDir:      m.bundleDir,
			BundleLoadMode: m.bundleLoadMode,
			OnOutput: func(p []byte) {
				m.appendTranscript("pty", p)
			},
			OnReady: func() {
				if m.bundleLoadMode {
					m.jobs.statusMu.Lock()
					m.jobs.status = StatusReady
					m.jobs.statusMu.Unlock()
					m.drawStatusBar()
				}
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

	return m.runWorkerMode()
}

// runWorkerMode runs the standard job-polling worker mode.
func (m *RootModel) runWorkerMode() error {
	// Register worker with the platform.
	name, metadata := worker.DefaultWorkerInfo()

	workerID, err := worker.Register(m.ctx, m.jobs.client, m.habitatID, m.jobs.instanceID, name, metadata, buildinfo.Version)
	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	m.jobs.workerID = workerID

	workerHeartbeatCtx, cancelWorkerHeartbeat := context.WithCancel(m.ctx)
	defer cancelWorkerHeartbeat()

	worker.StartHeartbeat(workerHeartbeatCtx, m.jobs.client, m.jobs.workerID, m.jobs.CurrentJobID, func(err error) {
		m.setLastError(fmt.Sprintf("Worker heartbeat failed: %v", err))
	})

	defer func() {
		jsnap := m.jobs.Snapshot()

		if err := worker.Deregister(m.jobs.client, m.jobs.workerID, jsnap.Completed, jsnap.Failed); err != nil {
			m.setLastError(fmt.Sprintf("Worker deregistration failed: %v", err))
		}
	}()

	// Start goroutines.
	var wg sync.WaitGroup

	// Terminal resize watcher (SIGWINCH + periodic reconciliation).
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.term.resizeLoop(m.ctx, m.done)
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

		m.jobs.Run(m.ctx, m.done)
	}()

	// Runner config refresh loop for MCP credential rotation.
	if hasRefreshableExecutor(m.executors) {
		wg.Add(1)

		go func() {
			defer wg.Done()

			m.jobs.RunnerConfigRefreshLoop(m.ctx, m.done)
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

	waitDone := make(chan struct{})

	go func() { wg.Wait(); close(waitDone) }()

	select {
	case <-waitDone:
	case <-time.After(m.ptyShutdownDeadline):
	}

	return nil
}

// runBundleLoadMode runs a single interactive session with bundle assets.
func (m *RootModel) runBundleLoadMode() error {
	// Start goroutines.
	var wg sync.WaitGroup

	// Terminal resize watcher.
	wg.Add(1)

	go func() {
		defer wg.Done()

		m.term.resizeLoop(m.ctx, m.done)
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

	waitDone := make(chan struct{})

	go func() { wg.Wait(); close(waitDone) }()

	select {
	case <-waitDone:
	case <-time.After(m.ptyShutdownDeadline):
	}

	return nil
}

func (m *RootModel) statusSnapshot() harnessstate.Snapshot {
	w, h := m.term.Dimensions()

	jsnap := m.jobs.Snapshot()

	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}

	now := nowFn()

	frame := layout.ComputeFrame(w, h, m.term.SidebarEnabled())

	return harnessstate.Snapshot{
		Width:              w,
		Height:             h,
		SidebarVisible:     frame.SidebarVisible,
		SidebarWidth:       frame.SidebarWidth,
		PaneXStart:         frame.PaneXStart,
		PaneWidth:          frame.PaneWidth,
		BundleLoadMode:     m.bundleLoadMode,
		BundleName:         m.bundleName,
		BundleVer:          m.bundleVer,
		BundleLayers:       m.bundleSummary.TotalLayers,
		BundleSkills:       append([]string(nil), m.bundleSummary.Skills...),
		BundleAgents:       append([]string(nil), m.bundleSummary.Agents...),
		BundleTools:        append([]string(nil), m.bundleSummary.ToolConfigs...),
		BundleOther:        append([]string(nil), m.bundleSummary.Other...),
		HabitatID:          m.habitatID,
		QueueID:            m.queueID,
		SupportedHarnesses: append([]string(nil), m.supportedHarnesses...),
		StatusLabel:        jsnap.StatusLabel,
		CopyMode:           m.isCopyMode(),
		JobID:              jsnap.JobID,
		LastHeartbeat:      jsnap.LastHeartbeat,
		Completed:          jsnap.Completed,
		Failed:             jsnap.Failed,
		LastError:          jsnap.LastError,
		LastErrorTime:      jsnap.LastErrorTime,
		MCPServers:         buildMCPServerStatuses(m.jobs, now),
		Now:                now,
	}
}

func (m *RootModel) drawStatusBar() {
	if m.term.AltScreenActive() {
		return
	}

	snap := m.statusSnapshot()
	m.term.WriteString(statusui.Render(&snap))
}

func (m *RootModel) shouldCaptureTranscript() bool {
	return hasTranscriptSource(m.supportedHarnesses)
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

// needsSignalDir checks if any supported harness type implements harnesstype.SignalDirConsumer.
func needsSignalDir(supportedHarnesses []string) bool {
	for _, name := range supportedHarnesses {
		info, ok := Lookup(name)
		if !ok {
			continue
		}

		executor := info.New()
		if _, ok := executor.(harnesstype.SignalDirConsumer); ok {
			return true
		}
	}

	return false
}

// hasTranscriptSource checks if any supported harness type implements harnesstype.TranscriptSource.
func hasTranscriptSource(supportedHarnesses []string) bool {
	for _, name := range supportedHarnesses {
		info, ok := Lookup(name)
		if !ok {
			continue
		}

		executor := info.New()
		if ts, ok := executor.(harnesstype.TranscriptSource); ok && ts.WantsTranscript() {
			return true
		}
	}

	return false
}

// hasRefreshableExecutor checks if any executor in the map implements harnesstype.Refreshable.
func hasRefreshableExecutor(executors map[string]harnesstype.Executor) bool {
	for _, executor := range executors {
		if _, ok := executor.(harnesstype.Refreshable); ok {
			return true
		}
	}

	return false
}

func (m *RootModel) ptyRowsForHeight(height int) int {
	return layout.PtyRowsForHeight(height)
}

// copyInput copies stdin to active executor with quit key handling.
// Uses Ctrl+Q (0x11) as quit key to avoid escape sequence ambiguity.
// Uses unix.Poll with a 100ms timeout so that blocking stdin reads
// can be interrupted when the context is canceled.
func (m *RootModel) copyInput() {
	// Replay any user keystrokes captured during the LR margin probe.
	if len(m.term.probeLeftoverInput) > 0 {
		leftover := m.term.probeLeftoverInput
		m.term.probeLeftoverInput = nil

		for _, harnessType := range m.supportedHarnesses {
			if executor, ok := m.executors[harnessType]; ok {
				if ir, ok := executor.(harnesstype.InputReceiver); ok {
					_, _ = ir.WriteInput(leftover)

					break
				}
			}
		}
	}

	stdinFD := int(os.Stdin.Fd())
	buf := make([]byte, 256)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Poll stdin with a short timeout so we can check ctx.Done()
		// periodically. os.Stdin.Read blocks indefinitely and is not
		// responsive to context cancellation (Go issue #7990).
		fds := []unix.PollFd{{Fd: int32(stdinFD), Events: unix.POLLIN}}

		n, err := unix.Poll(fds, 100) // 100ms timeout
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}

			return
		}

		if n == 0 {
			continue // timeout — loop back to check ctx
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
				if ir, ok := executor.(harnesstype.InputReceiver); ok {
					_, _ = ir.WriteInput(out)

					break
				}
			}
		}
	}
}

func (m *RootModel) handleCtrlC() bool {
	// When not actively running an interruptable job, Ctrl+C exits immediately.
	if !m.jobs.HasActiveInterruptableJob() {
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

	// Forward Ctrl+C to the current job's executor via InterruptHandler.
	harnessType := m.jobs.CurrentJobHarnessType()
	if executor, ok := m.executors[harnessType]; ok {
		if ih, ok := executor.(harnesstype.InterruptHandler); ok {
			_ = ih.Interrupt()
		}
	}

	m.infof("Interrupt sent to agent. Press Ctrl+C again within %s to exit watch mode.", window.Round(time.Second))

	return false
}

// updateStatusLoop periodically updates the status bar.
func (m *RootModel) updateStatusLoop() {
	ticker := time.NewTicker(5 * time.Second)
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

func (m *RootModel) infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	// Use \r\n because the terminal is in raw mode; writing through termWrite
	// ensures the output lands inside the scroll region alongside PTY output.
	m.term.Write([]byte(msg + "\r\n"))
}

// setLastError records an error to be displayed in the status bar.
func (m *RootModel) setLastError(msg string) {
	m.jobs.SetLastError(msg)
}

// isPTYActive reports whether any executor PTY is currently running, making it
// unsafe to send probe escape sequences to stdin/stdout. In bundle-load mode
// the PTY is always live; in worker mode it is live only while a job executes.
func (m *RootModel) isPTYActive() bool {
	if m.bundleLoadMode {
		return true
	}

	return m.jobs.CurrentJobID() != ""
}
