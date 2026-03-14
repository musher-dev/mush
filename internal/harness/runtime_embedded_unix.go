//go:build unix

package harness

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/hinshun/vt10x"
	"github.com/mattn/go-runewidth"

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

	copyMode    bool
	lastCtrlCAt time.Time

	scrollback   *scrollbackBuffer
	scrollOffset int // 0 = live view, >0 = lines scrolled back

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
	r.vt = vt10x.New(vt10x.WithSize(r.frame.PaneWidth, layout.PtyRowsForFrame(r.frame)))

	scrollbackCap := r.cfg.HarnessScrollbackLines()
	if scrollbackCap <= 0 {
		scrollbackCap = defaultScrollbackCapacity
	}

	r.scrollback = newScrollbackBuffer(scrollbackCap)

	screen.EnableMouse(tcell.MouseButtonEvents)

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
	ptyRows := layout.PtyRowsForFrame(r.frame)

	for _, harnessType := range r.supportedHarnesses {
		info, ok := Lookup(harnessType)
		if !ok {
			continue
		}

		executor := info.New()

		setupOpts := harnesstype.SetupOptions{
			TermWriter:     r,
			TermWidth:      r.frame.PaneWidth,
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

func (r *embeddedRuntime) handleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		r.signalDone()

		return true
	case tcell.KeyCtrlS:
		r.copyMode = !r.copyMode
		if r.copyMode {
			r.screen.DisableMouse()
		} else {
			r.screen.EnableMouse(tcell.MouseButtonEvents)
		}

		r.draw()

		return false
	case tcell.KeyCtrlC:
		if r.handleCtrlC() {
			return true
		}

		return false
	}

	if r.copyMode {
		if ev.Key() == tcell.KeyEscape {
			r.copyMode = false
			r.screen.EnableMouse(tcell.MouseButtonEvents)
			r.draw()
		}

		return false
	}

	// Scroll mode interactions — skip when alt-screen is active so that
	// full-screen apps (vim, less) receive PgUp/PgDn directly via the PTY.
	if !r.isAltScreenActive() {
		if r.scrollOffset > 0 {
			switch ev.Key() {
			case tcell.KeyPgUp:
				r.scrollUp(max(layout.PtyRowsForFrame(r.frame)-1, 1))
				return false
			case tcell.KeyPgDn:
				r.scrollDown(max(layout.PtyRowsForFrame(r.frame)-1, 1))
				return false
			case tcell.KeyEscape:
				r.uiMu.Lock()
				r.resetScroll()
				r.drawLocked()
				r.uiMu.Unlock()

				return false
			default:
				// Any forwarded key returns to live view.
				r.uiMu.Lock()
				r.resetScroll()
				r.drawLocked()
				r.uiMu.Unlock()
			}
		} else if ev.Key() == tcell.KeyPgUp {
			r.scrollUp(max(layout.PtyRowsForFrame(r.frame)-1, 1))
			return false
		}
	}

	keyBytes := encodeTCellKey(ev)
	if len(keyBytes) == 0 {
		return false
	}

	for _, harnessType := range r.supportedHarnesses {
		if executor, ok := r.executors[harnessType]; ok {
			if ir, ok := executor.(harnesstype.InputReceiver); ok {
				_, _ = ir.WriteInput(keyBytes)

				break
			}
		}
	}

	return false
}

func encodeTCellKey(ev *tcell.EventKey) []byte {
	switch ev.Key() {
	case tcell.KeyRune:
		ch := ev.Rune()
		buf := make([]byte, utf8.RuneLen(ch))
		utf8.EncodeRune(buf, ch)

		if ev.Modifiers()&tcell.ModAlt != 0 {
			return append([]byte{0x1b}, buf...)
		}

		return buf
	case tcell.KeyEnter:
		return []byte{'\r'}
	case tcell.KeyTab:
		return []byte{'\t'}
	case tcell.KeyBacktab:
		return []byte("\x1b[Z")
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return []byte{0x7f}
	case tcell.KeyEsc:
		return []byte{0x1b}
	case tcell.KeyUp:
		return []byte("\x1b[A")
	case tcell.KeyDown:
		return []byte("\x1b[B")
	case tcell.KeyRight:
		return []byte("\x1b[C")
	case tcell.KeyLeft:
		return []byte("\x1b[D")
	case tcell.KeyHome:
		return []byte("\x1b[H")
	case tcell.KeyEnd:
		return []byte("\x1b[F")
	case tcell.KeyPgUp:
		return []byte("\x1b[5~")
	case tcell.KeyPgDn:
		return []byte("\x1b[6~")
	case tcell.KeyDelete:
		return []byte("\x1b[3~")
	case tcell.KeyInsert:
		return []byte("\x1b[2~")
	case tcell.KeyCtrlA:
		return []byte{0x01}
	case tcell.KeyCtrlB:
		return []byte{0x02}
	case tcell.KeyCtrlD:
		return []byte{0x04}
	case tcell.KeyCtrlE:
		return []byte{0x05}
	case tcell.KeyCtrlF:
		return []byte{0x06}
	case tcell.KeyCtrlH:
		return []byte{0x08}
	case tcell.KeyCtrlI:
		return []byte{0x09}
	case tcell.KeyCtrlJ:
		return []byte{0x0a}
	case tcell.KeyCtrlK:
		return []byte{0x0b}
	case tcell.KeyCtrlL:
		return []byte{0x0c}
	case tcell.KeyCtrlM:
		return []byte{0x0d}
	case tcell.KeyCtrlN:
		return []byte{0x0e}
	case tcell.KeyCtrlO:
		return []byte{0x0f}
	case tcell.KeyCtrlP:
		return []byte{0x10}
	case tcell.KeyCtrlR:
		return []byte{0x12}
	case tcell.KeyCtrlT:
		return []byte{0x14}
	case tcell.KeyCtrlU:
		return []byte{0x15}
	case tcell.KeyCtrlV:
		return []byte{0x16}
	case tcell.KeyCtrlW:
		return []byte{0x17}
	case tcell.KeyCtrlX:
		return []byte{0x18}
	case tcell.KeyCtrlY:
		return []byte{0x19}
	case tcell.KeyCtrlZ:
		return []byte{0x1a}
	}

	return nil
}

func (r *embeddedRuntime) handleCtrlC() bool {
	if !r.jobs.HasActiveInterruptableJob() {
		r.signalDone()

		return true
	}

	nowFn := r.now
	if nowFn == nil {
		nowFn = time.Now
	}

	now := nowFn()
	secondPress := !r.lastCtrlCAt.IsZero() && now.Sub(r.lastCtrlCAt) <= r.ctrlCExitWindow

	if secondPress {
		r.lastCtrlCAt = time.Time{}
		r.infof("Second Ctrl+C received: exiting watch mode.")
		r.signalDone()

		return true
	}

	r.lastCtrlCAt = now

	if executor, ok := r.executors[r.jobs.CurrentJobHarnessType()]; ok {
		if ih, ok := executor.(harnesstype.InterruptHandler); ok {
			_ = ih.Interrupt()
		}
	}

	r.infof("Interrupt sent to agent. Press Ctrl+C again within %s to exit watch mode.", r.ctrlCExitWindow.Round(time.Second))

	return false
}

const scrollLinesPerTick = 3

func (r *embeddedRuntime) handleMouse(ev *tcell.EventMouse) {
	switch ev.Buttons() {
	case tcell.WheelUp:
		if r.isAltScreenActive() {
			return
		}

		x, y := ev.Position()
		if y < layout.TopBarHeight || (r.frame.SidebarVisible && x < r.frame.SidebarWidth) {
			return
		}

		r.scrollUp(scrollLinesPerTick)
	case tcell.WheelDown:
		if r.isAltScreenActive() {
			return
		}

		x, y := ev.Position()
		if y < layout.TopBarHeight || (r.frame.SidebarVisible && x < r.frame.SidebarWidth) {
			return
		}

		r.scrollDown(scrollLinesPerTick)
	case tcell.Button1:
		x, y := ev.Position()
		if r.frame.SidebarVisible && x < r.frame.SidebarWidth {
			sidebarRow := y - layout.TopBarHeight
			r.handleSidebarClick(sidebarRow)
		}
	}
}

func (r *embeddedRuntime) handleSidebarClick(row int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	for _, t := range r.sidebarClickTargets {
		if t.Row == row {
			r.sidebarExpanded[t.Section] = !r.sidebarExpanded[t.Section]
			r.drawLocked()

			return
		}
	}
}

func (r *embeddedRuntime) scrollUp(n int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	maxOffset := r.scrollback.Len()
	r.scrollOffset += n

	if r.scrollOffset > maxOffset {
		r.scrollOffset = maxOffset
	}

	r.drawLocked()
}

func (r *embeddedRuntime) scrollDown(n int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.scrollOffset -= n
	if r.scrollOffset < 0 {
		r.scrollOffset = 0
	}

	r.drawLocked()
}

func (r *embeddedRuntime) resetScroll() {
	r.scrollOffset = 0
}

// isAltScreenActive reports whether the virtual terminal is in alt-screen mode
// (e.g. vim, less). When true, scroll interception should be bypassed so that
// the child application receives navigation keys directly.
func (r *embeddedRuntime) isAltScreenActive() bool {
	return r.vt.Mode()&vt10x.ModeAltScreen != 0
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

func (r *embeddedRuntime) handleResize(width, height int) {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	width, height = clampTerminalSize(width, height)
	r.width, r.height = width, height
	r.frame = layout.ComputeFrame(width, height, true)
	r.vt.Resize(r.frame.PaneWidth, layout.PtyRowsForFrame(r.frame))

	rows := layout.PtyRowsForFrame(r.frame)
	for _, executor := range r.executors {
		if rs, ok := executor.(harnesstype.Resizable); ok {
			rs.Resize(rows, r.frame.PaneWidth)
		}
	}

	r.screen.Clear()
	r.drawLocked()
}

func (r *embeddedRuntime) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	// Capture rows before write to detect scroll-off.
	r.captureScrolledLines(p)

	r.drawLocked()

	return len(p), nil
}

// captureScrolledLines snapshots visible rows before vt.Write, writes to vt,
// then detects which top rows scrolled off and pushes them to scrollback.
func (r *embeddedRuntime) captureScrolledLines(p []byte) {
	// Skip capture when alt screen is active (e.g. vim, less).
	if r.vt.Mode()&vt10x.ModeAltScreen != 0 {
		_, _ = r.vt.Write(p)
		return
	}

	rows := layout.PtyRowsForFrame(r.frame)
	cols := r.frame.PaneWidth

	// Snapshot all visible rows before write.
	before := make([][]vt10x.Glyph, rows)

	r.vt.Lock()

	for row := 0; row < rows; row++ {
		line := make([]vt10x.Glyph, cols)
		for col := 0; col < cols; col++ {
			line[col] = r.vt.Cell(col, row)
		}

		before[row] = line
	}

	r.vt.Unlock()

	_, _ = r.vt.Write(p)

	// Snapshot rows after write.
	after := make([][]vt10x.Glyph, rows)

	r.vt.Lock()

	for row := 0; row < rows; row++ {
		line := make([]vt10x.Glyph, cols)
		for col := 0; col < cols; col++ {
			line[col] = r.vt.Cell(col, row)
		}

		after[row] = line
	}

	r.vt.Unlock()

	// Find how many rows scrolled off by matching the longest suffix of
	// "before" that appears as a prefix of "after".
	scrolledOff := 0

	for shift := 1; shift < rows; shift++ {
		if glyphRowsEqual(before[shift:], after[:rows-shift], cols) {
			scrolledOff = shift
		}
	}

	// Push scrolled-off rows into the scrollback buffer.
	for i := 0; i < scrolledOff; i++ {
		r.scrollback.Push(before[i])
	}

	// Anchor scroll position so the user keeps viewing the same content.
	if scrolledOff > 0 && r.scrollOffset > 0 {
		r.scrollOffset += scrolledOff
		if r.scrollOffset > r.scrollback.Len() {
			r.scrollOffset = r.scrollback.Len()
		}
	}
}

// glyphRowsEqual compares two slices of glyph rows for character equality.
func glyphRowsEqual(left, right [][]vt10x.Glyph, cols int) bool {
	if len(left) != len(right) {
		return false
	}

	for row := range left {
		for col := 0; col < cols && col < len(left[row]) && col < len(right[row]); col++ {
			if left[row][col].Char != right[row][col].Char {
				return false
			}
		}
	}

	return true
}

func (r *embeddedRuntime) draw() {
	r.uiMu.Lock()
	defer r.uiMu.Unlock()

	r.drawLocked()
}

func (r *embeddedRuntime) drawLocked() {
	if r.screen == nil {
		return
	}

	r.renderTopBar()
	r.renderSidebar()
	r.renderViewport()
	r.screen.Show()
}

type styledSpan struct {
	text  string
	style tcell.Style
}

func (r *embeddedRuntime) renderTopBar() {
	barStyle := tcell.StyleDefault.Background(tnSurface).Foreground(tnText)

	for col := 0; col < r.width; col++ {
		r.screen.SetContent(col, 0, ' ', nil, barStyle)
	}

	snap := r.statusSnapshot()

	accentStyle := barStyle.Foreground(tnAccent).Bold(true)
	statusColor := statusTCellColor(snap.StatusLabel)
	statusStyle := barStyle.Foreground(statusColor).Bold(true)

	mode := "LIVE"
	modeStyle := barStyle.Foreground(tnSuccess)

	if r.scrollOffset > 0 {
		mode = fmt.Sprintf("SCROLL -%d", r.scrollOffset)
		modeStyle = barStyle.Foreground(tnAccent)
	} else if snap.CopyMode {
		mode = "COPY"
		modeStyle = barStyle.Foreground(tnWarning)
	}

	spans := []styledSpan{
		{"MUSH", accentStyle},
		{"  Status: ", barStyle},
		{snap.StatusLabel, statusStyle},
		{"  Mode: ", barStyle},
		{mode, modeStyle},
		{fmt.Sprintf("  OK:%d Fail:%d", snap.Completed, snap.Failed), barStyle},
	}

	if snap.JobID != "" {
		spans = append(spans, styledSpan{"  Job: " + snap.JobID, barStyle})
	}

	right := "^C Int | ^S Copy | ^Q Quit"

	// Calculate left width for fitting.
	leftWidth := 0
	for _, span := range spans {
		leftWidth += runewidth.StringWidth(span.text)
	}

	// Render left spans.
	col := 0

	for _, span := range spans {
		for _, ch := range span.text {
			if col >= r.width {
				break
			}

			r.screen.SetContent(col, 0, ch, nil, span.style)
			col += runewidth.RuneWidth(ch)
		}
	}

	// Render right-aligned hints.
	rightWidth := runewidth.StringWidth(right)
	rightStart := r.width - rightWidth

	if rightStart > leftWidth {
		hintCol := rightStart

		for _, ch := range right {
			if hintCol >= r.width {
				break
			}

			r.screen.SetContent(hintCol, 0, ch, nil, barStyle)
			hintCol += runewidth.RuneWidth(ch)
		}
	}
}

func statusTCellColor(label string) tcell.Color {
	switch label {
	case "Ready", "Connected":
		return tnSuccess
	case "Starting...", "Processing":
		return tnWarning
	case "Error":
		return tnError
	default:
		return tnText
	}
}

func (r *embeddedRuntime) renderSidebar() {
	if !r.frame.SidebarVisible {
		return
	}

	sideStyle := tcell.StyleDefault.Background(tnBorder).Foreground(tnText)
	borderStyle := tcell.StyleDefault.Background(tnSurface).Foreground(tnMuted)

	lines := r.sidebarLines(layout.PtyRowsForFrame(r.frame))

	for row := 0; row < layout.PtyRowsForFrame(r.frame); row++ {
		screenY := layout.TopBarHeight + row

		for col := 0; col < r.frame.SidebarWidth; col++ {
			r.screen.SetContent(col, screenY, ' ', nil, sideStyle)
		}

		line := ""
		if row < len(lines) {
			line = lines[row]
		}

		line = runewidth.Truncate(line, r.frame.SidebarWidth-1, "")
		line += strings.Repeat(" ", max(0, r.frame.SidebarWidth-runewidth.StringWidth(line)))

		col := 0
		for _, ch := range line {
			if col >= r.frame.SidebarWidth {
				break
			}

			r.screen.SetContent(col, screenY, ch, nil, sideStyle)
			col += runewidth.RuneWidth(ch)
		}

		r.screen.SetContent(r.frame.SidebarWidth, screenY, '│', nil, borderStyle)
	}
}

func (r *embeddedRuntime) sidebarLines(rows int) []string {
	snap := r.statusSnapshot()
	lines, targets := statusui.SidebarLines(&snap, rows)
	r.sidebarClickTargets = targets

	return lines
}

func (r *embeddedRuntime) renderViewport() {
	rows := layout.PtyRowsForFrame(r.frame)
	paneX := r.frame.PaneXStart - 1
	paneY := r.frame.ContentTop - 1
	clearStyle := tcell.StyleDefault.Background(tnPTYBg).Foreground(tnText)

	for row := 0; row < rows; row++ {
		for col := 0; col < r.frame.PaneWidth; col++ {
			r.screen.SetContent(paneX+col, paneY+row, ' ', nil, clearStyle)
		}
	}

	if r.scrollOffset > 0 {
		r.renderScrolledViewport(rows, paneX, paneY)
		return
	}

	r.vt.Lock()
	defer r.vt.Unlock()

	for row := 0; row < rows; row++ {
		for col := 0; col < r.frame.PaneWidth; col++ {
			glyph := r.vt.Cell(col, row)

			ch := glyph.Char
			if ch == 0 {
				ch = ' '
			}

			style := tcell.StyleDefault.
				Foreground(vtColorToTCell(glyph.FG, true)).
				Background(vtColorToTCell(glyph.BG, false))
			r.screen.SetContent(paneX+col, paneY+row, ch, nil, style)
		}
	}

	cursor := r.vt.Cursor()
	inBounds := cursor.Y >= 0 && cursor.Y < rows &&
		cursor.X >= 0 && cursor.X < r.frame.PaneWidth

	if inBounds && r.vt.CursorVisible() {
		screenX := paneX + cursor.X
		screenY := paneY + cursor.Y

		r.applySoftwareCursor(screenX, screenY)
		r.screen.ShowCursor(screenX, screenY)
	} else {
		r.screen.HideCursor()
	}
}

// applySoftwareCursor renders a reverse-video cell at the given screen position
// to provide a visible cursor regardless of terminal emulator or hardware cursor state.
func (r *embeddedRuntime) applySoftwareCursor(screenX, screenY int) {
	content, style, _ := r.screen.Get(screenX, screenY)

	ch := ' '

	var combc []rune

	if content != "" {
		runes := []rune(content)

		ch = runes[0]
		if len(runes) > 1 {
			combc = runes[1:]
		}
	}

	fg, bg, attrs := style.Decompose()
	reversed := tcell.StyleDefault.Foreground(bg).Background(fg).Attributes(attrs)
	r.screen.SetContent(screenX, screenY, ch, combc, reversed)
}

// renderScrolledViewport renders the viewport from a mix of scrollback buffer
// and live vt state when the user has scrolled back.
func (r *embeddedRuntime) renderScrolledViewport(rows, paneX, paneY int) {
	r.screen.HideCursor()

	// The viewport shows rows from (scrollOffset-1) down to (scrollOffset-rows)
	// in the scrollback, with any remaining rows from the live vt top.
	//
	// scrollOffset=1 means the top row of viewport is scrollback line 0 (newest)
	// and the rest are from live vt rows 0..(rows-2).
	//
	// More generally:
	// - scrollback lines shown: min(scrollOffset, rows) from scrollback
	// - live vt rows shown: rows - scrollbackLinesShown, starting from vt row 0

	scrollbackLinesShown := r.scrollOffset
	if scrollbackLinesShown > rows {
		scrollbackLinesShown = rows
	}

	liveLinesShown := rows - scrollbackLinesShown

	// Render scrollback lines at the top of the viewport.
	for row := 0; row < scrollbackLinesShown; row++ {
		// scrollOffset lines from newest: we want offset (scrollOffset-1) at row 0,
		// (scrollOffset-2) at row 1, etc.
		sbOffset := r.scrollOffset - 1 - row
		cells := r.scrollback.Line(sbOffset)

		for col := 0; col < r.frame.PaneWidth; col++ {
			ch := ' '
			style := tcell.StyleDefault.Background(tnPTYBg).Foreground(tnText)

			if cells != nil && col < len(cells) {
				ch = cells[col].Char
				if ch == 0 {
					ch = ' '
				}

				style = tcell.StyleDefault.
					Foreground(vtColorToTCell(cells[col].FG, true)).
					Background(vtColorToTCell(cells[col].BG, false))
			}

			r.screen.SetContent(paneX+col, paneY+row, ch, nil, style)
		}
	}

	// Render live vt rows below the scrollback lines.
	if liveLinesShown > 0 {
		r.vt.Lock()
		defer r.vt.Unlock()

		for row := 0; row < liveLinesShown; row++ {
			screenRow := scrollbackLinesShown + row

			for col := 0; col < r.frame.PaneWidth; col++ {
				glyph := r.vt.Cell(col, row)

				ch := glyph.Char
				if ch == 0 {
					ch = ' '
				}

				style := tcell.StyleDefault.
					Foreground(vtColorToTCell(glyph.FG, true)).
					Background(vtColorToTCell(glyph.BG, false))
				r.screen.SetContent(paneX+col, paneY+screenRow, ch, nil, style)
			}
		}
	}
}

func vtColorToTCell(color vt10x.Color, isForeground bool) tcell.Color {
	if color == vt10x.DefaultFG {
		if isForeground {
			return tnText
		}

		return tnPTYBg
	}

	if color == vt10x.DefaultBG {
		if isForeground {
			return tnText
		}

		return tnPTYBg
	}

	if color < 16 {
		palette := []tcell.Color{
			tcell.ColorBlack, tcell.ColorMaroon, tcell.ColorGreen, tcell.ColorOlive,
			tcell.ColorNavy, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorSilver,
			tcell.ColorGray, tcell.ColorRed, tcell.ColorLime, tcell.ColorYellow,
			tcell.ColorBlue, tcell.ColorFuchsia, tcell.ColorAqua, tcell.ColorWhite,
		}

		return palette[int(color)]
	}

	if color < 256 {
		return tcell.PaletteColor(int(color))
	}

	rgb := int(color)
	red := (rgb >> 16) & 0xff
	green := (rgb >> 8) & 0xff
	blue := rgb & 0xff

	return tcell.NewRGBColor(int32(red), int32(green), int32(blue))
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
		PaneWidth:          frame.PaneWidth,
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
		CopyMode:           r.copyMode,
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
