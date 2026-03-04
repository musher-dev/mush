//go:build unix

package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
)

// CopilotExecutor runs jobs via GitHub Copilot CLI.
// Each queued job runs as a one-shot process. Bundle mode starts an interactive PTY session.
type CopilotExecutor struct {
	opts SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	pgid       int
	waitDoneCh chan struct{}

	mcpConfigPath   string
	mcpConfigSig    string
	mcpConfigRemove func() error
}

func init() {
	Register(Info{
		Name:      "copilot",
		Available: AvailableFunc("copilot"),
		New:       func() Executor { return &CopilotExecutor{} },
		MCPSpec: &MCPSpec{
			Def:         mustGetProvider("copilot").MCP,
			BuildConfig: BuildJSONMCPConfig,
		},
	})
}

// Setup stores options and starts interactive mode for bundle sessions.
func (e *CopilotExecutor) Setup(ctx context.Context, opts *SetupOptions) error {
	e.opts = *opts

	if _, err := exec.LookPath("copilot"); err != nil {
		return fmt.Errorf("copilot CLI not found in PATH")
	}

	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			return err
		}
	}

	if opts.BundleDir != "" {
		if err := e.startInteractive(ctx, opts); err != nil {
			e.cleanupMCPConfigFile()
			return err
		}
	}

	if opts.OnReady != nil {
		opts.OnReady()
	}

	return nil
}

// Execute runs a one-shot copilot command and returns normalized output.
func (e *CopilotExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &ExecError{
			Reason:  "execution_error",
			Message: "copilot interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := getPromptFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	args := []string{"-p", prompt, "-s", "--allow-all-tools", "--json"}
	if mcpArgs := e.mcpConfigArgs(); len(mcpArgs) > 0 {
		args = append(args, mcpArgs...)
	}

	cmd := exec.CommandContext(ctx, "copilot", args...) //nolint:gosec // G204: command originates from trusted job execution payload

	if job.Execution != nil && job.Execution.WorkingDirectory != "" {
		cmd.Dir = job.Execution.WorkingDirectory
	}

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

	var output bytes.Buffer

	outWriter := io.Writer(&output)
	if e.opts.TermWriter != nil {
		outWriter = io.MultiWriter(e.opts.TermWriter, &output)
	}

	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	rawOutput := strings.TrimSpace(output.String())
	parsedOutput := parseCopilotJSONOutput(rawOutput)
	fallbackOutput := ansi.Strip(rawOutput)

	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, &ExecError{Reason: "timeout", Message: "copilot execution timed out", Retry: true}
			}

			return nil, &ExecError{
				Reason:  "execution_error",
				Message: fmt.Sprintf("copilot execution canceled: %v", ctxErr),
				Retry:   true,
			}
		}

		exitCode := 1

		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		msg := fmt.Sprintf("copilot exited with code %d", exitCode)
		if fallbackOutput != "" {
			msg = fmt.Sprintf("%s: %s", msg, fallbackOutput)
		}

		return nil, &ExecError{
			Reason:  "execution_error",
			Message: msg,
			Retry:   true,
		}
	}

	resultOutput := parsedOutput
	if strings.TrimSpace(resultOutput) == "" {
		resultOutput = fallbackOutput
	}

	return &ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     resultOutput,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

// Reset is a no-op for copilot one-shot worker jobs.
func (e *CopilotExecutor) Reset(_ context.Context) error {
	return nil
}

// WriteInput forwards terminal input to the interactive Copilot process.
func (e *CopilotExecutor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to copilot pty: %w", err)
	}

	return n, nil
}

// NeedsRefresh implements Refreshable.
func (e *CopilotExecutor) NeedsRefresh(cfg *client.RunnerConfigResponse) bool {
	specs := BuildMCPProviderSpecs(cfg, time.Now())

	sig, err := MCPSignature(specs)
	if err != nil {
		return true
	}

	return sig != e.mcpConfigSig
}

// ApplyRefresh implements Refreshable.
func (e *CopilotExecutor) ApplyRefresh(_ context.Context, cfg *client.RunnerConfigResponse) error {
	return e.applyRunnerConfig(cfg)
}

func (e *CopilotExecutor) applyRunnerConfig(cfg *client.RunnerConfigResponse) error {
	now := time.Now()

	info, ok := Lookup("copilot")
	if !ok || info.MCPSpec == nil {
		return nil
	}

	path, sig, cleanup, err := CreateMCPConfigFile(info.MCPSpec, cfg, now)
	if err != nil {
		return err
	}

	oldCleanup := e.mcpConfigRemove
	e.mcpConfigPath = path
	e.mcpConfigSig = sig
	e.mcpConfigRemove = cleanup

	if oldCleanup != nil {
		_ = oldCleanup()
	}

	return nil
}

func (e *CopilotExecutor) startInteractive(ctx context.Context, opts *SetupOptions) error {
	spec, _ := GetProvider("copilot")

	var args []string
	if opts.BundleDir != "" && spec != nil && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, opts.BundleDir)
	}

	if mcpArgs := e.mcpConfigArgs(); len(mcpArgs) > 0 {
		args = append(args, mcpArgs...)
	}

	cmd := exec.CommandContext(ctx, "copilot", args...) //nolint:gosec // G204: args from controlled input

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		return fmt.Errorf("start copilot interactive session: %w", err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.ptmx = ptmx
	e.pgid = 0

	if cmd.Process != nil && cmd.Process.Pid > 0 {
		if pgid, pgErr := syscall.Getpgid(cmd.Process.Pid); pgErr == nil {
			e.pgid = pgid
		}
	}

	e.waitDoneCh = make(chan struct{})
	waitDoneCh := e.waitDoneCh
	e.mu.Unlock()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				if opts.TermWriter != nil {
					_, _ = opts.TermWriter.Write(buf[:n])
				}

				if opts.OnOutput != nil {
					opts.OnOutput(buf[:n])
				}
			}

			if readErr != nil {
				return
			}
		}
	}()

	go func() {
		_ = cmd.Wait()

		close(waitDoneCh)

		if opts.OnExit != nil {
			opts.OnExit()
		}
	}()

	return nil
}

// Teardown stops the interactive Copilot process and cleans up MCP temp files.
func (e *CopilotExecutor) Teardown() {
	e.mu.Lock()
	cmd := e.cmd
	ptmx := e.ptmx
	pgid := e.pgid
	waitDoneCh := e.waitDoneCh
	e.cmd = nil
	e.ptmx = nil
	e.pgid = 0
	e.waitDoneCh = nil
	e.mu.Unlock()

	stopInteractiveProcess(cmd, ptmx, pgid, waitDoneCh)

	e.cleanupMCPConfigFile()
}

func (e *CopilotExecutor) cleanupMCPConfigFile() {
	if e.mcpConfigRemove == nil {
		return
	}

	_ = e.mcpConfigRemove()
	e.mcpConfigRemove = nil
	e.mcpConfigPath = ""
}

func (e *CopilotExecutor) mcpConfigArgs() []string {
	spec, ok := GetProvider("copilot")
	if !ok || spec == nil || spec.CLI == nil || spec.CLI.MCPConfig == "" || e.mcpConfigPath == "" {
		return nil
	}

	return []string{spec.CLI.MCPConfig, "@" + e.mcpConfigPath}
}

func parseCopilotJSONOutput(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	var textParts []string

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		collectCopilotText(event, &textParts)
	}

	return strings.TrimSpace(strings.Join(textParts, "\n"))
}

func collectCopilotText(node any, textParts *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				*textParts = append(*textParts, trimmed)
			}
		}

		for _, key := range []string{"content", "message", "response", "output", "value"} {
			child, ok := v[key]
			if !ok {
				continue
			}

			collectCopilotText(child, textParts)
		}
	case []any:
		for _, child := range v {
			collectCopilotText(child, textParts)
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			*textParts = append(*textParts, trimmed)
		}
	}
}

var (
	_ Executor      = (*CopilotExecutor)(nil)
	_ InputReceiver = (*CopilotExecutor)(nil)
	_ Refreshable   = (*CopilotExecutor)(nil)
)
