//go:build unix

package opencode

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
	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// Executor runs jobs via OpenCode CLI.
// Each job runs in its own process. Bundle mode starts an interactive TUI session.
type Executor struct {
	opts harnesstype.SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	pgid       int
	waitDoneCh chan struct{}

	mcpConfigSig     string
	mcpConfigContent string
}

// Setup stores options. OpenCode has no persistent process outside bundle mode.
func (e *Executor) Setup(ctx context.Context, opts *harnesstype.SetupOptions) error {
	e.opts = *opts

	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode CLI not found in PATH")
	}

	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			return err
		}
	}

	// Bundle mode is an interactive, long-lived OpenCode TUI session.
	if opts.BundleDir != "" {
		if err := e.startInteractive(ctx, opts); err != nil {
			return err
		}
	}

	if opts.OnReady != nil {
		opts.OnReady()
	}

	return nil
}

// Execute runs an opencode run command and returns the result.
func (e *Executor) Execute(ctx context.Context, job *client.Job) (*harnesstype.ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &harnesstype.ExecError{
			Reason:  "execution_error",
			Message: "opencode interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := harnesstype.GetPromptFromJob(job)
	if err != nil {
		return nil, &harnesstype.ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	args := []string{"run", "--format", "json", prompt}
	cmd := exec.CommandContext(ctx, "opencode", args...) //nolint:gosec // G204: command originates from trusted job execution payload

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

	if e.mcpConfigContent != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENCODE_CONFIG_CONTENT=%s", e.mcpConfigContent))
	}

	var (
		output    bytes.Buffer
		outWriter io.Writer = &output
	)

	if e.opts.TermWriter != nil {
		outWriter = io.MultiWriter(e.opts.TermWriter, &output)
	}

	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	rawOutput := strings.TrimSpace(output.String())
	parsedOutput, eventErrs, parsedAny := parseOpenCodeJSONOutput(rawOutput)
	fallbackOutput := ansi.Strip(rawOutput)

	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, &harnesstype.ExecError{Reason: "timeout", Message: "opencode execution timed out", Retry: true}
			}

			return nil, &harnesstype.ExecError{
				Reason:  "execution_error",
				Message: fmt.Sprintf("opencode execution canceled: %v", ctxErr),
				Retry:   true,
			}
		}

		exitCode := 1

		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		msg := fmt.Sprintf("opencode exited with code %d", exitCode)
		if len(eventErrs) > 0 {
			msg = fmt.Sprintf("%s: %s", msg, strings.Join(eventErrs, "; "))
		} else if fallbackOutput != "" {
			msg = fmt.Sprintf("%s: %s", msg, fallbackOutput)
		}

		return nil, &harnesstype.ExecError{
			Reason:  "execution_error",
			Message: msg,
			Retry:   true,
		}
	}

	if len(eventErrs) > 0 {
		return nil, &harnesstype.ExecError{
			Reason:  "execution_error",
			Message: strings.Join(eventErrs, "; "),
			Retry:   true,
		}
	}

	resultOutput := parsedOutput
	if !parsedAny || strings.TrimSpace(resultOutput) == "" {
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

// Reset is a no-op for opencode (each worker job is a separate process).
func (e *Executor) Reset(_ context.Context) error {
	return nil
}

// Resize implements Resizable for interactive bundle sessions.
func (e *Executor) Resize(rows, cols int) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return
	}

	_ = pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// WriteInput forwards terminal input to the interactive OpenCode process.
func (e *Executor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to opencode pty: %w", err)
	}

	return n, nil
}

// Interrupt implements InterruptHandler for interactive bundle sessions.
func (e *Executor) Interrupt() error {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return nil
	}

	_, err := ptmx.Write([]byte{0x03}) // Ctrl+C
	if err != nil {
		return fmt.Errorf("interrupt opencode pty: %w", err)
	}

	return nil
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

	specs := harnesstype.BuildMCPProviderSpecs(cfg, now)

	sig, err := harnesstype.MCPSignature(specs)
	if err != nil {
		return fmt.Errorf("mcp signature: %w", err)
	}

	content, err := mcpSpec.BuildConfig(specs)
	if err != nil {
		return fmt.Errorf("build mcp config: %w", err)
	}

	e.mcpConfigSig = sig
	e.mcpConfigContent = string(content)

	return nil
}

func (e *Executor) startInteractive(ctx context.Context, opts *harnesstype.SetupOptions) error {
	args := []string{}
	if opts.BundleDir != "" {
		args = append(args, opts.BundleDir)
	}

	cmd := exec.CommandContext(ctx, "opencode", args...) //nolint:gosec // G204: args from controlled input

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	cmd.Env = append(cmd.Env, opts.Env...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	if e.mcpConfigContent != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENCODE_CONFIG_CONTENT=%s", e.mcpConfigContent))
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		return fmt.Errorf("start opencode interactive session: %w", err)
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

// Teardown stops the interactive opencode process when running in bundle mode.
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
}

func parseOpenCodeJSONOutput(raw string) (output string, eventErrs []string, parsed bool) {
	if strings.TrimSpace(raw) == "" {
		return "", nil, false
	}

	var (
		textParts []string
		errs      []string
		parsedAny bool
	)

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

		parsedAny = true

		eventType, _ := event["type"].(string)
		switch eventType {
		case "text":
			part, _ := event["part"].(map[string]any)
			text, _ := part["text"].(string)

			text = strings.TrimSpace(text)
			if text != "" {
				textParts = append(textParts, text)
			}
		case "error":
			if msg := openCodeErrorMessage(event["error"]); msg != "" {
				errs = append(errs, msg)
			}
		}
	}

	return strings.Join(textParts, "\n"), errs, parsedAny
}

func openCodeErrorMessage(raw any) string {
	obj, ok := raw.(map[string]any)
	if !ok {
		return ""
	}

	if data, ok := obj["data"].(map[string]any); ok {
		if msg, ok := data["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}

	if msg, ok := obj["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}

	if name, ok := obj["name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}

	return ""
}

var (
	_ harnesstype.Executor         = (*Executor)(nil)
	_ harnesstype.InputReceiver    = (*Executor)(nil)
	_ harnesstype.Resizable        = (*Executor)(nil)
	_ harnesstype.InterruptHandler = (*Executor)(nil)
	_ harnesstype.Refreshable      = (*Executor)(nil)
)
