//go:build unix

package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
)

// ClaudeExecutor runs jobs via Claude Code in a persistent PTY session.
type ClaudeExecutor struct {
	mu   sync.Mutex
	ptmx *os.File
	cmd  *exec.Cmd
	pgid int

	opts SetupOptions

	// promptDetected is signaled when Claude is ready for input.
	promptDetected  chan struct{}
	readyForJob     bool
	lastPromptSeen  time.Time
	promptConfirmed bool
	bypassAccepted  bool

	// Output capture during job execution.
	captureMu    sync.Mutex
	outputBuffer bytes.Buffer
	capturing    bool

	// Signal directory for completion detection.
	signalDir string

	// MCP config management.
	mcpConfigPath   string
	mcpConfigSig    string
	mcpConfigRemove func() error
	loadedMCPNames  []string
	runnerConfig    *client.RunnerConfigResponse

	// PTY injection helpers (injectable for tests).
	setPTYSize       func(*os.File, *pty.Winsize) error
	startPTYWithSize func(*exec.Cmd, *pty.Winsize) (*os.File, error)
	startPTYFunc     func(context.Context) error
	startOutputFunc  func()
	waitForReadyFunc func(context.Context)

	// ptyReady delivers active PTY handles to the output reader loop.
	ptyReady chan *os.File

	// done signals executor shutdown.
	done     chan struct{}
	doneOnce sync.Once

	// hooks restore function.
	restoreHooks func() error

	// shutdown deadline for PTY process.
	ptyShutdownDeadline time.Duration

	// outputReaderDone signals that the output reader goroutine has finished.
	outputReaderDone chan struct{}
}

func init() {
	Register(Info{
		Name:      "claude",
		Available: AvailableFunc("claude"),
		New:       func() Executor { return NewClaudeExecutor() },
		MCPSpec: &MCPSpec{
			Def:         mustGetProvider("claude").MCP,
			BuildConfig: BuildJSONMCPConfig,
		},
	})
}

func mustGetProvider(name string) *ProviderSpec {
	spec, ok := GetProvider(name)
	if !ok {
		panic(fmt.Sprintf("harness: provider %q not found", name))
	}

	return spec
}

// NewClaudeExecutor creates a new ClaudeExecutor with default settings.
func NewClaudeExecutor() *ClaudeExecutor {
	executor := &ClaudeExecutor{
		promptDetected:      make(chan struct{}, 1),
		ptyReady:            make(chan *os.File, 4),
		done:                make(chan struct{}),
		outputReaderDone:    make(chan struct{}),
		setPTYSize:          pty.Setsize,
		startPTYWithSize:    pty.StartWithSize,
		ptyShutdownDeadline: defaultPTYShutdownDeadline,
	}

	executor.startPTYFunc = executor.startPTY
	executor.startOutputFunc = func() {
		go executor.copyPTYOutput()
	}
	executor.waitForReadyFunc = executor.waitForReady

	return executor
}

// Setup initializes the Claude executor: signal dir, stop hook, MCP config, PTY.
func (e *ClaudeExecutor) Setup(ctx context.Context, opts *SetupOptions) error {
	e.opts = *opts
	e.signalDir = opts.SignalDir

	// Install Stop hook for completion signaling.
	if e.signalDir != "" {
		restoreHooks, err := installStopHook(e.signalDir)
		if err != nil {
			return err
		}

		e.restoreHooks = restoreHooks
	}

	// Build ephemeral Claude MCP config from runner config.
	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			// Non-fatal: log via output and continue.
			if opts.OnOutput != nil {
				opts.OnOutput([]byte(fmt.Sprintf("MCP config disabled: %v\r\n", err)))
			}
		}
	}

	// Start the PTY.
	startPTY := e.startPTYFunc
	if startPTY == nil {
		startPTY = e.startPTY
	}

	if err := startPTY(ctx); err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	startOutput := e.startOutputFunc
	if startOutput == nil {
		startOutput = func() {
			go e.copyPTYOutput()
		}
	}

	startOutput()

	waitForReady := e.waitForReadyFunc
	if waitForReady == nil {
		waitForReady = e.waitForReady
	}

	if opts.BundleLoadMode {
		go func() {
			waitForReady(ctx)

			if opts.OnReady != nil {
				opts.OnReady()
			}
		}()
	} else {
		// Link mode blocks until prompt detection confirms readiness.
		waitForReady(ctx)

		if opts.OnReady != nil {
			opts.OnReady()
		}
	}

	return nil
}

// Execute injects a prompt, waits for completion, and returns the result.
func (e *ClaudeExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	prompt, err := getPromptFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	// Clear any prior signal file and record current job.
	if e.signalDir != "" {
		_ = os.Remove(e.signalPath())
		_ = os.WriteFile(e.currentJobPath(), []byte(job.ID), 0o600)
	}

	// Start capturing output.
	e.captureMu.Lock()
	e.capturing = true
	e.outputBuffer.Reset()
	e.readyForJob = false
	e.captureMu.Unlock()

	// Inject the prompt into the PTY.
	e.injectPrompt(prompt)

	startedAt := time.Now()

	// Wait for completion signal with timeout.
	output, execErr := e.waitForSignalFile(ctx)
	duration := time.Since(startedAt)

	if execErr != nil {
		reason := "execution_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}

		return nil, &ExecError{Reason: reason, Message: execErr.Error(), Retry: true}
	}

	return &ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     output,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

// Reset sends /clear and waits for the prompt to reappear.
func (e *ClaudeExecutor) Reset(ctx context.Context) error {
	// Clean up signal/job files.
	if e.signalDir != "" {
		_ = os.Remove(e.currentJobPath())
		_ = os.Remove(e.signalPath())
	}

	e.sendClear()

	// Drain and wait for prompt.
	e.drainPromptDetected()

	select {
	case <-ctx.Done():
	case <-e.done:
	case <-e.promptDetected:
	case <-time.After(10 * time.Second):
	}

	time.Sleep(1 * time.Second) // Settle time.

	return nil
}

// Teardown stops the PTY and restores hooks.
func (e *ClaudeExecutor) Teardown() {
	e.doneOnce.Do(func() {
		close(e.done)
	})

	e.closePTY()

	// Wait for output reader to finish.
	select {
	case <-e.outputReaderDone:
	case <-time.After(2 * time.Second):
	}

	e.cleanupMCPConfigFile()

	if e.restoreHooks != nil {
		_ = e.restoreHooks()
		e.restoreHooks = nil
	}
}

// Resize implements Resizable.
func (e *ClaudeExecutor) Resize(rows, cols int) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return
	}

	setSize := e.setPTYSize
	if setSize == nil {
		setSize = pty.Setsize
	}

	_ = setSize(ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// WriteInput implements InputReceiver.
func (e *ClaudeExecutor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to pty: %w", err)
	}

	return n, nil
}

// NeedsRefresh implements Refreshable.
func (e *ClaudeExecutor) NeedsRefresh(cfg *client.RunnerConfigResponse) bool {
	specs := BuildMCPProviderSpecs(cfg, time.Now())

	sig, err := MCPSignature(specs)
	if err != nil {
		return false
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return sig != e.mcpConfigSig
}

// ApplyRefresh implements Refreshable.
func (e *ClaudeExecutor) ApplyRefresh(ctx context.Context, cfg *client.RunnerConfigResponse) error {
	oldNames := e.loadedMCPNames

	if err := e.applyRunnerConfig(cfg); err != nil {
		slog.Default().Error(
			"MCP config refresh failed",
			slog.String("component", "mcp"),
			slog.String("event.type", "mcp.reload.error"),
			slog.String("error", err.Error()),
		)

		return err
	}

	e.closePTY()

	if err := e.startPTY(ctx); err != nil {
		return err
	}

	e.waitForReady(ctx)

	newNames := e.loadedMCPNames
	if !sameStringSlice(oldNames, newNames) && e.opts.OnOutput != nil {
		msg := fmt.Sprintf("MCP servers reloaded: %s\r\n", summarizeMCPServers(newNames))
		e.opts.OnOutput([]byte(msg))
	}

	slog.Default().Info(
		"MCP servers reloaded",
		slog.String("component", "mcp"),
		slog.String("event.type", "mcp.reload"),
		slog.Int("mcp.server_count", len(newNames)),
		slog.Any("mcp.server_names", newNames),
	)

	return nil
}

// --- Internal methods ---

func (e *ClaudeExecutor) startPTY(ctx context.Context) error {
	args := e.commandArgs()
	slog.Default().Debug(
		"starting harness PTY",
		slog.String("component", "harness"),
		slog.String("event.type", "harness.pty.start"),
		slog.Any("harness.args", args),
	)

	cmd := exec.CommandContext(ctx, "claude", args...) //nolint:gosec // args are entirely controlled by our code

	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"FORCE_COLOR=1",
		"MUSH_SIGNAL_DIR="+e.signalDir,
	)

	startWithSize := e.startPTYWithSize
	if startWithSize == nil {
		startWithSize = pty.StartWithSize
	}

	ptmx, err := startWithSize(cmd, &pty.Winsize{
		Rows: uint16(e.opts.TermHeight),
		Cols: uint16(e.opts.TermWidth),
	})
	if err != nil {
		return annotateStartPTYError(err, cmd.Path)
	}

	e.mu.Lock()
	e.ptmx = ptmx
	e.cmd = cmd
	e.pgid = 0

	if cmd.Process != nil && cmd.Process.Pid > 0 {
		if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil {
			e.pgid = pgid
		}
	}
	e.mu.Unlock()

	// Drain stale handles before delivering the new one.
	for len(e.ptyReady) > 0 {
		<-e.ptyReady
	}

	e.ptyReady <- ptmx

	return nil
}

func (e *ClaudeExecutor) commandArgs() []string {
	spec, _ := GetProvider("claude")

	var args []string
	if !e.opts.BundleLoadMode {
		args = append(args, "--dangerously-skip-permissions")
	}

	if e.opts.BundleDir != "" && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, e.opts.BundleDir)
	}

	if e.mcpConfigPath != "" && spec.CLI != nil && spec.CLI.MCPConfig != "" {
		args = append(args, spec.CLI.MCPConfig, e.mcpConfigPath)
	}

	return args
}

func (e *ClaudeExecutor) closePTY() {
	e.mu.Lock()
	ptmx := e.ptmx
	cmd := e.cmd
	pgid := e.pgid
	e.ptmx = nil
	e.cmd = nil
	e.pgid = 0
	e.mu.Unlock()

	if ptmx != nil {
		_ = ptmx.Close()
	}

	if cmd == nil || cmd.Process == nil {
		return
	}

	slog.Default().Debug(
		"stopping harness PTY",
		slog.String("component", "harness"),
		slog.String("event.type", "harness.pty.stop"),
	)

	waitCh := make(chan error, 1)

	go func() {
		waitCh <- cmd.Wait()
	}()

	sendSignal(cmd.Process.Pid, pgid, syscall.SIGTERM)

	deadline := e.ptyShutdownDeadline
	if deadline <= 0 {
		deadline = defaultPTYShutdownDeadline
	}

	select {
	case <-waitCh:
		return
	case <-time.After(deadline):
		sendSignal(cmd.Process.Pid, pgid, syscall.SIGKILL)

		select {
		case <-waitCh:
		case <-time.After(deadline):
		}
	}
}

func sendSignal(pid, pgid int, sig syscall.Signal) {
	if pgid > 0 {
		if err := syscall.Kill(-pgid, sig); err == nil || errors.Is(err, syscall.ESRCH) {
			return
		}
	}

	if pid <= 0 {
		return
	}

	_ = syscall.Kill(pid, sig)
}

func (e *ClaudeExecutor) activePTY() *os.File {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.ptmx
}

func (e *ClaudeExecutor) copyPTYOutput() {
	defer close(e.outputReaderDone)

	for {
		select {
		case <-e.done:
			return
		case ptmx := <-e.ptyReady:
			if ptmx == nil {
				continue
			}

			e.readPTYOutput(ptmx)
		}
	}
}

func (e *ClaudeExecutor) readPTYOutput(ptmx *os.File) {
	buf := make([]byte, 4096)
	promptRing := make([]byte, len(PromptDetectionBytes))
	promptRingIdx := 0

	var dialogBuf bytes.Buffer

	for {
		select {
		case <-e.done:
			return
		default:
		}

		bytesRead, err := ptmx.Read(buf)
		if err != nil {
			return
		}

		if bytesRead <= 0 {
			continue
		}

		// Write to terminal output.
		if e.opts.TermWriter != nil {
			_, _ = e.opts.TermWriter.Write(buf[:bytesRead])
		}

		if e.opts.OnOutput != nil {
			e.opts.OnOutput(buf[:bytesRead])
		}

		// Detect bypass dialog and auto-accept (only in link mode where
		// --dangerously-skip-permissions triggers a trust dialog).
		if !e.opts.BundleLoadMode {
			e.captureMu.Lock()
			if !e.bypassAccepted {
				dialogBuf.Write(buf[:bytesRead])

				if bytes.Contains(dialogBuf.Bytes(), []byte("Esc to cancel")) {
					e.bypassAccepted = true
					e.captureMu.Unlock()
					dialogBuf.Reset()

					go func() {
						time.Sleep(300 * time.Millisecond)

						if active := e.activePTY(); active != nil {
							_, _ = active.WriteString("\x1b[B")

							time.Sleep(100 * time.Millisecond)

							_, _ = active.WriteString("\r")
						}
					}()
				} else {
					e.captureMu.Unlock()
				}
			} else {
				e.captureMu.Unlock()
			}
		}

		// Capture output if we're processing a job.
		e.captureMu.Lock()
		if e.capturing {
			e.outputBuffer.Write(buf[:bytesRead])
		}

		e.promptConfirmed = false
		e.captureMu.Unlock()

		// Detect prompt pattern.
		for i := 0; i < bytesRead; i++ {
			promptRing[promptRingIdx] = buf[i]
			promptRingIdx = (promptRingIdx + 1) % len(PromptDetectionBytes)

			if checkPromptMatch(promptRing, promptRingIdx) {
				e.onPromptPatternSeen()
			}
		}
	}
}

func checkPromptMatch(ring []byte, idx int) bool {
	for i := 0; i < len(PromptDetectionBytes); i++ {
		ringIdx := (idx + i) % len(ring)
		if ring[ringIdx] != PromptDetectionBytes[i] {
			return false
		}
	}

	return true
}

func (e *ClaudeExecutor) onPromptPatternSeen() {
	e.captureMu.Lock()
	e.lastPromptSeen = time.Now()
	e.captureMu.Unlock()

	go func() {
		time.Sleep(PromptDebounceTime)

		e.captureMu.Lock()

		timeSincePrompt := time.Since(e.lastPromptSeen)
		if timeSincePrompt >= PromptDebounceTime-10*time.Millisecond && !e.promptConfirmed {
			e.promptConfirmed = true
			e.captureMu.Unlock()
			e.onPromptConfirmed()
		} else {
			e.captureMu.Unlock()
		}
	}()
}

func (e *ClaudeExecutor) onPromptConfirmed() {
	e.captureMu.Lock()
	e.readyForJob = true
	e.captureMu.Unlock()

	select {
	case e.promptDetected <- struct{}{}:
	default:
	}
}

func (e *ClaudeExecutor) waitForReady(ctx context.Context) {
	e.drainPromptDetected()

	select {
	case <-ctx.Done():
	case <-e.done:
	case <-e.promptDetected:
	case <-time.After(15 * time.Second):
		e.captureMu.Lock()
		bypassed := e.bypassAccepted
		e.captureMu.Unlock()

		if bypassed {
			time.Sleep(2 * time.Second)
		}
	}
}

func (e *ClaudeExecutor) drainPromptDetected() {
	for {
		select {
		case <-e.promptDetected:
		default:
			return
		}
	}
}

func (e *ClaudeExecutor) injectPrompt(prompt string) {
	ptmx := e.activePTY()
	if ptmx == nil {
		return
	}

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

	time.Sleep(PTYPasteSettleDelay)

	_, _ = ptmx.WriteString("\r")
}

func (e *ClaudeExecutor) waitForSignalFile(ctx context.Context) (string, error) {
	ticker := time.NewTicker(SignalPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wait for signal file canceled: %w", ctx.Err())
		case <-e.done:
			return "", errors.New("harness stopped")
		case <-ticker.C:
			if _, err := os.Stat(e.signalPath()); err != nil {
				continue
			}

			_ = os.Remove(e.signalPath())

			e.captureMu.Lock()
			e.capturing = false
			output := ansi.Strip(e.outputBuffer.String())
			e.outputBuffer.Reset()
			e.captureMu.Unlock()

			return output, nil
		}
	}
}

func (e *ClaudeExecutor) sendClear() {
	ptmx := e.activePTY()
	if ptmx == nil {
		return
	}

	time.Sleep(500 * time.Millisecond)

	_, _ = ptmx.WriteString("/clear")

	time.Sleep(PTYPostWriteDelay)

	_, _ = ptmx.WriteString("\r")
}

func (e *ClaudeExecutor) signalPath() string {
	if e.signalDir == "" {
		return ""
	}

	return e.signalDir + "/" + SignalFileName
}

func (e *ClaudeExecutor) currentJobPath() string {
	if e.signalDir == "" {
		return ""
	}

	return e.signalDir + "/current-job"
}

func (e *ClaudeExecutor) applyRunnerConfig(cfg *client.RunnerConfigResponse) error {
	now := time.Now()

	info, ok := Lookup("claude")
	if !ok || info.MCPSpec == nil {
		return nil
	}

	path, sig, cleanup, err := CreateMCPConfigFile(info.MCPSpec, cfg, now)
	if err != nil {
		return err
	}

	names := LoadedMCPProviderNames(cfg, now)

	oldCleanup := e.mcpConfigRemove
	e.mcpConfigPath = path
	e.mcpConfigSig = sig
	e.mcpConfigRemove = cleanup
	e.loadedMCPNames = names
	e.runnerConfig = cfg

	if oldCleanup != nil {
		_ = oldCleanup()
	}

	slog.Default().Info(
		"MCP config applied",
		slog.String("component", "mcp"),
		slog.String("event.type", "mcp.config.applied"),
		slog.Int("mcp.server_count", len(names)),
		slog.Any("mcp.server_names", names),
	)

	return nil
}

func (e *ClaudeExecutor) cleanupMCPConfigFile() {
	if e.mcpConfigRemove == nil {
		return
	}

	_ = e.mcpConfigRemove()
	e.mcpConfigRemove = nil
	e.mcpConfigPath = ""
}

// getPromptFromJob extracts the prompt from a job's data and execution config.
func getPromptFromJob(job *client.Job) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}

	if rendered := job.GetRenderedInstruction(); rendered != "" {
		return rendered, nil
	}

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

// Ensure ClaudeExecutor satisfies all interfaces.
var (
	_ Executor      = (*ClaudeExecutor)(nil)
	_ Resizable     = (*ClaudeExecutor)(nil)
	_ InputReceiver = (*ClaudeExecutor)(nil)
	_ Refreshable   = (*ClaudeExecutor)(nil)
	_ io.Writer     = (*lockedWriter)(nil)
)
