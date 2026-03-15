//go:build unix

package harness

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/hinshun/vt10x"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
	harnessstate "github.com/musher-dev/mush/internal/harness/state"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
	statusui "github.com/musher-dev/mush/internal/harness/ui/status"
	"github.com/musher-dev/mush/internal/transcript"
	"github.com/musher-dev/mush/internal/worker"
)

// Tokyo Night palette for harness chrome.
// Dark-mode values matching internal/tui/nav/styles.go.
var (
	tnSurface = tcell.NewRGBColor(0x2A, 0x2E, 0x3A) // colorSurface dark
	tnBorder  = tcell.NewRGBColor(0x3B, 0x42, 0x52) // colorBorder dark
	tnAccent  = tcell.NewRGBColor(0x9D, 0x7C, 0xD8) // colorAccent dark
	tnText    = tcell.NewRGBColor(0xC8, 0xCE, 0xDB) // colorText dark
	tnMuted   = tcell.NewRGBColor(0x4E, 0x56, 0x68) // colorMuted dark
	tnPTYBg   = tcell.NewRGBColor(0x1A, 0x1B, 0x26) // Tokyo Night storm base
	tnSuccess = tcell.NewRGBColor(0x9E, 0xCE, 0x6A) // colorSuccess dark
	tnWarning = tcell.NewRGBColor(0xE0, 0xAF, 0x68) // colorWarning dark
	tnError   = tcell.NewRGBColor(0xF7, 0x76, 0x8E) // colorError dark
)

// DefaultExecutionTimeout is the fallback when no execution timeout is set on the job.
const DefaultExecutionTimeout = 10 * time.Minute

const (
	defaultCtrlCExitWindow     = 2 * time.Second
	defaultPTYShutdownDeadline = 3 * time.Second
)

type embeddedRuntime struct {
	ctx    context.Context
	cancel context.CancelFunc

	screen tcell.Screen
	vt     vt10x.Terminal

	uiMu sync.Mutex

	width  int
	height int
	frame  layout.Frame

	mouseCaptureEnabled bool
	lastCtrlCAt         time.Time

	scrollback        *scrollbackBuffer
	viewportTop       int
	followTail        bool
	historyNotice     string
	scrollbarDragging bool
	scrollbarDragY    int

	jobs      *JobLoop
	executors map[string]harnesstype.Executor

	cfg                *config.Config
	supportedHarnesses []string
	habitatID          string
	queueID            string

	transcriptEnabled bool
	transcriptDir     string
	transcriptLines   int
	transcriptStore   *transcript.Store
	transcriptMu      sync.Mutex

	bundleLoadMode bool
	bundleName     string
	bundleVer      string
	bundleDir      string
	bundleWorkDir  string
	bundleEnv      []string
	bundleSummary  BundleSummary

	sidebarExpanded     map[string]bool
	sidebarClickTargets []statusui.SidebarClickTarget

	done      chan struct{}
	closeOnce sync.Once

	now             func() time.Time
	ctrlCExitWindow time.Duration
}

func runEmbeddedHarness(ctx context.Context, cfg *Config) error {
	r := newEmbeddedRuntime(ctx, cfg)

	return r.Run()
}

func newEmbeddedRuntime(ctx context.Context, cfg *Config) *embeddedRuntime {
	ctx, cancel := context.WithCancel(ctx)

	initialStatus := StatusConnecting
	if cfg.BundleLoadMode {
		initialStatus = StatusStarting
	}

	executors := make(map[string]harnesstype.Executor)
	loadedCfg := config.Load()

	r := &embeddedRuntime{
		ctx:                ctx,
		cancel:             cancel,
		executors:          executors,
		cfg:                loadedCfg,
		supportedHarnesses: cfg.SupportedHarnesses,
		habitatID:          cfg.HabitatID,
		queueID:            cfg.QueueID,
		transcriptEnabled:  cfg.TranscriptEnabled,
		transcriptDir:      cfg.TranscriptDir,
		transcriptLines:    cfg.TranscriptLines,
		bundleLoadMode:     cfg.BundleLoadMode,
		bundleName:         cfg.BundleName,
		bundleVer:          cfg.BundleVer,
		bundleDir:          cfg.BundleDir,
		bundleWorkDir:      cfg.BundleWorkDir,
		bundleEnv:          append([]string(nil), cfg.BundleEnv...),
		bundleSummary:      cfg.BundleSummary,
		sidebarExpanded:    make(map[string]bool),
		done:               make(chan struct{}),
		now:                time.Now,
		ctrlCExitWindow:    defaultCtrlCExitWindow,
		followTail:         true,
	}

	r.jobs = &JobLoop{
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
	}

	r.jobs.drawStatusBar = r.draw
	r.jobs.infof = r.infof
	r.jobs.signalDone = r.signalDone
	r.jobs.now = r.now

	return r
}

func (r *embeddedRuntime) Run() error {
	if r.jobs.client == nil && !r.bundleLoadMode {
		return fmt.Errorf("missing client in harness config")
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("init terminal screen: %w", err)
	}

	if err := screen.Init(); err != nil {
		return fmt.Errorf("initialize terminal screen: %w", err)
	}

	r.screen = screen

	defer screen.Fini()

	width, height := screen.Size()
	width, height = clampTerminalSize(width, height)
	r.width, r.height = width, height
	r.frame = layout.ComputeFrame(width, height, true)
	r.vt = vt10x.New(vt10x.WithSize(r.frame.ViewportWidth, layout.PtyRowsForFrame(&r.frame)))

	scrollbackCap := r.cfg.HarnessScrollbackLines()
	if scrollbackCap <= 0 {
		scrollbackCap = defaultScrollbackCapacity
	}

	r.scrollback = newScrollbackBuffer(scrollbackCap)

	historyEnabled := r.transcriptEnabled
	if !historyEnabled {
		historyEnabled = r.cfg.HistoryEnabled()
	}

	if historyEnabled && hasTranscriptSource(r.supportedHarnesses) {
		historyDir := r.transcriptDir
		if historyDir == "" {
			historyDir = r.cfg.HistoryDir()
		}

		historyLines := r.transcriptLines
		if historyLines <= 0 {
			historyLines = r.cfg.HistoryScrollbackLines()
		}

		store, tErr := transcript.NewStore(transcript.StoreOptions{
			SessionID: uuid.NewString(),
			Dir:       historyDir,
			MaxLines:  historyLines,
		})
		if tErr != nil {
			r.jobs.SetLastError(fmt.Sprintf("Transcript disabled: %v", tErr))
		} else {
			r.transcriptMu.Lock()
			r.transcriptStore = store
			r.transcriptMu.Unlock()

			defer r.closeTranscript()
		}
	}

	if needsSignalDir(r.supportedHarnesses) {
		signalDir, mkErr := os.MkdirTemp("", "mush-signals-")
		if mkErr != nil {
			return fmt.Errorf("failed to create signal directory: %w", mkErr)
		}

		r.jobs.signalDir = signalDir

		defer func() { _ = os.RemoveAll(signalDir) }()
	}

	if err := r.setupExecutors(); err != nil {
		return err
	}

	defer func() {
		for _, executor := range r.executors {
			executor.Teardown()
		}
	}()

	r.draw()

	if r.bundleLoadMode {
		return r.runBundleLoadMode()
	}

	return r.runWorkerMode()
}

func (r *embeddedRuntime) setupExecutors() error {
	ptyRows := layout.PtyRowsForFrame(&r.frame)

	for _, harnessType := range r.supportedHarnesses {
		info, ok := Lookup(harnessType)
		if !ok {
			continue
		}

		executor := info.New()

		setupOpts := harnesstype.SetupOptions{
			TermWriter:     r,
			TermWidth:      r.frame.ViewportWidth,
			TermHeight:     ptyRows,
			SignalDir:      r.jobs.signalDir,
			RunnerConfig:   r.jobs.runnerConfig,
			BundleDir:      r.bundleDir,
			WorkingDir:     r.bundleWorkDir,
			Env:            append([]string(nil), r.bundleEnv...),
			BundleLoadMode: r.bundleLoadMode,
			OnOutput: func(p []byte) {
				r.appendTranscript("pty", p)
			},
			OnReady: func() {
				if r.bundleLoadMode {
					r.jobs.statusMu.Lock()
					r.jobs.status = StatusReady
					r.jobs.statusMu.Unlock()
					r.draw()
				}
			},
			OnExit: r.signalDone,
		}

		if err := executor.Setup(r.ctx, &setupOpts); err != nil {
			return fmt.Errorf("failed to setup %s executor: %w", harnessType, err)
		}

		r.executors[harnessType] = executor
	}

	return nil
}

func (r *embeddedRuntime) runWorkerMode() error {
	name, metadata := worker.DefaultWorkerInfo()

	workerID, err := worker.Register(r.ctx, r.jobs.client, r.habitatID, r.jobs.instanceID, name, metadata, buildinfo.Version)
	if err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}

	r.jobs.workerID = workerID

	workerHeartbeatCtx, cancelWorkerHeartbeat := context.WithCancel(r.ctx)
	defer cancelWorkerHeartbeat()

	worker.StartHeartbeat(workerHeartbeatCtx, r.jobs.client, r.jobs.workerID, r.jobs.CurrentJobID, func(err error) {
		r.jobs.SetLastError(fmt.Sprintf("Worker heartbeat failed: %v", err))
		r.draw()
	})

	defer func() {
		jsnap := r.jobs.Snapshot()
		if err := worker.Deregister(r.jobs.client, r.jobs.workerID, jsnap.Completed, jsnap.Failed); err != nil {
			r.jobs.SetLastError(fmt.Sprintf("Worker deregistration failed: %v", err))
		}
	}()

	var wg sync.WaitGroup

	wg.Add(1)

	go func() { defer wg.Done(); r.eventLoop() }()

	wg.Add(1)

	go func() { defer wg.Done(); r.updateStatusLoop() }()

	wg.Add(1)

	go func() { defer wg.Done(); r.jobs.Run(r.ctx, r.done) }()

	if hasRefreshableExecutor(r.executors) {
		wg.Add(1)

		go func() { defer wg.Done(); r.jobs.RunnerConfigRefreshLoop(r.ctx, r.done) }()
	}

	go func() {
		select {
		case <-r.ctx.Done():
			r.signalDone()
		case <-r.done:
		}
	}()

	<-r.done
	r.cancel()

	waitDone := make(chan struct{})

	go func() { wg.Wait(); close(waitDone) }()

	select {
	case <-waitDone:
	case <-time.After(defaultPTYShutdownDeadline):
	}

	return nil
}

func (r *embeddedRuntime) runBundleLoadMode() error {
	var wg sync.WaitGroup

	wg.Add(1)

	go func() { defer wg.Done(); r.eventLoop() }()

	wg.Add(1)

	go func() { defer wg.Done(); r.updateStatusLoop() }()

	go func() {
		select {
		case <-r.ctx.Done():
			r.signalDone()
		case <-r.done:
		}
	}()

	<-r.done
	r.cancel()

	waitDone := make(chan struct{})

	go func() { wg.Wait(); close(waitDone) }()

	select {
	case <-waitDone:
	case <-time.After(defaultPTYShutdownDeadline):
	}

	return nil
}

func (r *embeddedRuntime) eventLoop() {
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		ev := r.screen.PollEvent()
		if ev == nil {
			continue
		}

		switch msg := ev.(type) {
		case *tcell.EventResize:
			width, height := msg.Size()
			r.handleResize(width, height)
		case *tcell.EventKey:
			if r.handleKey(msg) {
				return
			}
		case *tcell.EventMouse:
			r.handleMouse(msg)
		}
	}
}

func (r *embeddedRuntime) updateStatusLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.done:
			return
		case <-ticker.C:
			r.draw()
		}
	}
}

func (r *embeddedRuntime) statusSnapshot() harnessstate.Snapshot {
	jsnap := r.jobs.Snapshot()

	nowFn := r.now
	if nowFn == nil {
		nowFn = time.Now
	}

	now := nowFn()
	frame := r.frame

	return harnessstate.Snapshot{
		Width:              r.width,
		Height:             r.height,
		SidebarVisible:     frame.SidebarVisible,
		SidebarWidth:       frame.SidebarWidth,
		PaneXStart:         frame.PaneXStart,
		PaneWidth:          frame.ViewportWidth,
		BundleLoadMode:     r.bundleLoadMode,
		BundleName:         r.bundleName,
		BundleVer:          r.bundleVer,
		BundleLayers:       r.bundleSummary.TotalLayers,
		BundleSkills:       append([]string(nil), r.bundleSummary.Skills...),
		BundleAgents:       append([]string(nil), r.bundleSummary.Agents...),
		BundleTools:        append([]string(nil), r.bundleSummary.ToolConfigs...),
		BundleOther:        append([]string(nil), r.bundleSummary.Other...),
		HabitatID:          r.habitatID,
		QueueID:            r.queueID,
		SupportedHarnesses: append([]string(nil), r.supportedHarnesses...),
		StatusLabel:        jsnap.StatusLabel,
		JobID:              jsnap.JobID,
		LastHeartbeat:      jsnap.LastHeartbeat,
		Completed:          jsnap.Completed,
		Failed:             jsnap.Failed,
		LastError:          jsnap.LastError,
		LastErrorTime:      jsnap.LastErrorTime,
		MCPServers:         buildMCPServerStatuses(r.jobs, now),
		ExpandedSections:   r.sidebarExpanded,
		Now:                now,
	}
}

func (r *embeddedRuntime) appendTranscript(stream string, chunk []byte) {
	r.transcriptMu.Lock()
	store := r.transcriptStore
	r.transcriptMu.Unlock()

	if store == nil || len(chunk) == 0 {
		return
	}

	if err := store.Append(stream, chunk); err != nil {
		r.jobs.SetLastError(fmt.Sprintf("Transcript write failed: %v", err))
	}
}

func (r *embeddedRuntime) closeTranscript() {
	r.transcriptMu.Lock()
	store := r.transcriptStore
	r.transcriptStore = nil
	r.transcriptMu.Unlock()

	if store == nil {
		return
	}

	if err := store.Close(); err != nil {
		r.jobs.SetLastError(fmt.Sprintf("Transcript close failed: %v", err))
	}
}

func (r *embeddedRuntime) infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_, _ = r.Write([]byte(msg + "\r\n"))
}

func (r *embeddedRuntime) signalDone() {
	r.closeOnce.Do(func() { close(r.done) })
}

func clampTerminalSize(width, height int) (clampedWidth, clampedHeight int) {
	return layout.ClampTerminalSize(width, height)
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
