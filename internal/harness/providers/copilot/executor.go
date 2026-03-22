//go:build unix

package copilot

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
	"time"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/executil"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// Executor runs jobs via GitHub Copilot CLI.
// Each queued job runs as a one-shot process. Bundle mode starts an interactive PTY session.
type Executor struct {
	opts harnesstype.SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	pgid       int
	waitDoneCh chan struct{}

	mcpConfigPath        string
	mcpConfigSig         string
	mcpConfigRemove      func() error
	startInteractiveFunc func(context.Context, *harnesstype.SetupOptions) error
}

// Setup stores options and starts interactive mode for bundle sessions.
func (e *Executor) Setup(ctx context.Context, opts *harnesstype.SetupOptions) error {
	e.opts = *opts

	if _, err := executil.LookPath("copilot"); err != nil {
		return fmt.Errorf("copilot CLI not found in PATH")
	}

	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			return err
		}
	}

	if opts.BundleDir != "" {
		startInteractive := e.startInteractiveFunc
		if startInteractive == nil {
			startInteractive = e.startInteractive
		}

		if err := startInteractive(ctx, opts); err != nil {
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
func (e *Executor) Execute(ctx context.Context, job *client.Job) (*harnesstype.ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &harnesstype.ExecError{
			Reason:  "execution_error",
			Message: "copilot interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := harnesstype.GetPromptFromJob(job)
	if err != nil {
		return nil, &harnesstype.ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	args := []string{"-p", prompt, "-s", "--allow-all-tools", "--json"}
	if mcpArgs := e.mcpConfigArgs(); len(mcpArgs) > 0 {
		args = append(args, mcpArgs...)
	}

	cmd, err := executil.CommandContext(ctx, "copilot", args...)
	if err != nil {
		return nil, &harnesstype.ExecError{Reason: "execution_error", Message: err.Error()}
	}

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
		fmt.Sprintf("MUSHER_JOB_ID=%s", job.ID),
		fmt.Sprintf("MUSHER_JOB_NAME=%s", job.GetDisplayName()),
		fmt.Sprintf("MUSHER_JOB_QUEUE=%s", job.QueueID),
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
		return nil, copilotRunError(ctx, runErr, fallbackOutput)
	}

	resultOutput := parsedOutput
	if strings.TrimSpace(resultOutput) == "" {
		resultOutput = fallbackOutput
	}

	return &harnesstype.ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     resultOutput,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

func copilotRunError(ctx context.Context, runErr error, fallbackOutput string) *harnesstype.ExecError {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return &harnesstype.ExecError{Reason: "timeout", Message: "copilot execution timed out", Retry: true}
		}

		return &harnesstype.ExecError{
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

	return &harnesstype.ExecError{
		Reason:  "execution_error",
		Message: copilotExitMessage(exitCode, fallbackOutput),
		Retry:   true,
	}
}

func copilotExitMessage(exitCode int, fallbackOutput string) string {
	msg := fmt.Sprintf("copilot exited with code %d", exitCode)
	if strings.TrimSpace(fallbackOutput) == "" {
		return msg
	}

	return fmt.Sprintf("%s: %s", msg, fallbackOutput)
}

// Reset is a no-op for copilot one-shot worker jobs.
func (e *Executor) Reset(_ context.Context) error {
	return nil
}

// WriteInput forwards terminal input to the interactive Copilot process.
func (e *Executor) WriteInput(p []byte) (int, error) {
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
func (e *Executor) NeedsRefresh(cfg *client.RunnerConfigResponse) bool {
	specs := harnesstype.BuildMCPProviderSpecs(cfg, time.Now())

	sig, err := harnesstype.MCPSignature(specs)
	if err != nil {
		return true
	}

	return sig != e.mcpConfigSig
}

// ApplyRefresh implements Refreshable.
func (e *Executor) ApplyRefresh(_ context.Context, cfg *client.RunnerConfigResponse) error {
	return e.applyRunnerConfig(cfg)
}

func (e *Executor) applyRunnerConfig(cfg *client.RunnerConfigResponse) error {
	now := time.Now()

	mcpSpec := Module.MCPSpec
	if mcpSpec == nil {
		return nil
	}

	path, sig, cleanup, err := harnesstype.CreateMCPConfigFile(mcpSpec, cfg, now)
	if err != nil {
		return fmt.Errorf("create mcp config: %w", err)
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

func (e *Executor) startInteractive(ctx context.Context, opts *harnesstype.SetupOptions) error {
	spec := Module.Spec

	var args []string
	if opts.BundleDir != "" && spec != nil && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, opts.BundleDir)
	}

	if mcpArgs := e.mcpConfigArgs(); len(mcpArgs) > 0 {
		args = append(args, mcpArgs...)
	}

	cmd, err := executil.CommandContext(ctx, "copilot", args...)
	if err != nil {
		return fmt.Errorf("resolve copilot command: %w", err)
	}

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	cmd.Env = append(cmd.Env, opts.Env...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	ptmx, pgid, waitDoneCh, err := harnesstype.StartInteractiveProcess(cmd, opts, opts.OnExit)
	if err != nil {
		return fmt.Errorf("start copilot interactive session: %w", err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.ptmx = ptmx
	e.pgid = pgid
	e.waitDoneCh = waitDoneCh
	e.mu.Unlock()

	return nil
}

// Teardown stops the interactive Copilot process and cleans up MCP temp files.
func (e *Executor) Teardown() {
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

	harnesstype.StopInteractiveProcess(cmd, ptmx, pgid, waitDoneCh)

	e.cleanupMCPConfigFile()
}

func (e *Executor) cleanupMCPConfigFile() {
	if e.mcpConfigRemove == nil {
		return
	}

	_ = e.mcpConfigRemove()
	e.mcpConfigRemove = nil
	e.mcpConfigPath = ""
}

func (e *Executor) mcpConfigArgs() []string {
	spec := Module.Spec
	if spec == nil || spec.CLI == nil || spec.CLI.MCPConfig == "" || e.mcpConfigPath == "" {
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
	_ harnesstype.Executor      = (*Executor)(nil)
	_ harnesstype.InputReceiver = (*Executor)(nil)
	_ harnesstype.Refreshable   = (*Executor)(nil)
)
