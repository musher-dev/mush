//go:build unix

package gemini

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// Executor runs jobs via Gemini CLI.
// Each queued job runs as a one-shot process. Bundle mode starts an interactive PTY session.
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

// Setup stores options and starts interactive mode for bundle sessions.
func (e *Executor) Setup(ctx context.Context, opts *harnesstype.SetupOptions) error {
	e.opts = *opts

	if _, err := exec.LookPath("gemini"); err != nil {
		return fmt.Errorf("gemini CLI not found in PATH")
	}

	if opts.RunnerConfig != nil {
		if err := e.applyRunnerConfig(opts.RunnerConfig); err != nil {
			return err
		}
	}

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

// Execute runs a one-shot gemini command and returns normalized output.
func (e *Executor) Execute(ctx context.Context, job *client.Job) (*harnesstype.ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &harnesstype.ExecError{
			Reason:  "execution_error",
			Message: "gemini interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := harnesstype.GetPromptFromJob(job)
	if err != nil {
		return nil, &harnesstype.ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	args := []string{
		"--approval-mode", "yolo",
		"--sandbox", "workspace-write",
		"--output-format", "text",
		"-p", prompt,
	}

	cmd := exec.CommandContext(ctx, "gemini", args...) //nolint:gosec // G204: command originates from trusted job execution payload

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

	cleanup, env, err := buildGeminiConfigEnv(e.mcpConfigContent)
	if err != nil {
		return nil, &harnesstype.ExecError{Reason: "execution_error", Message: err.Error()}
	}

	if cleanup != nil {
		defer func() {
			_ = cleanup()
		}()
	}

	cmd.Env = append(cmd.Env, env...)

	var output strings.Builder

	outWriter := io.Writer(&output)
	if e.opts.TermWriter != nil {
		outWriter = io.MultiWriter(e.opts.TermWriter, &output)
	}

	cmd.Stdout = outWriter
	cmd.Stderr = outWriter

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	if runErr != nil {
		return nil, harnesstype.HandleOneShotRunError(ctx, runErr, output.String(), "gemini")
	}

	resultOutput := ansi.Strip(strings.TrimSpace(output.String()))

	return &harnesstype.ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     resultOutput,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

// Reset is a no-op for gemini one-shot worker jobs.
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

// WriteInput forwards terminal input to the interactive Gemini process.
func (e *Executor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to gemini pty: %w", err)
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
		return fmt.Errorf("interrupt gemini pty: %w", err)
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
	args := []string{
		"--approval-mode", "yolo",
		"--sandbox", "workspace-write",
	}

	cmd := exec.CommandContext(ctx, "gemini", args...)
	if opts.BundleDir != "" {
		cmd.Dir = opts.BundleDir
	}

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	cleanup, env, err := buildGeminiConfigEnv(e.mcpConfigContent)
	if err != nil {
		return err
	}

	cmd.Env = append(cmd.Env, env...)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		if cleanup != nil {
			_ = cleanup()
		}

		return fmt.Errorf("start gemini interactive session: %w", err)
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

		if cleanup != nil {
			_ = cleanup()
		}

		if opts.OnExit != nil {
			opts.OnExit()
		}
	}()

	return nil
}

// Teardown stops the interactive Gemini process when running in bundle mode.
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

func buildGeminiConfigEnv(configContent string) (cleanup func() error, env []string, err error) {
	if strings.TrimSpace(configContent) == "" {
		return nil, nil, nil
	}

	configDir, err := os.MkdirTemp("", "mush-gemini-config-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create gemini config dir: %w", err)
	}

	settingsPath := filepath.Join(configDir, "settings.json")
	if writeErr := os.WriteFile(settingsPath, []byte(configContent), 0o600); writeErr != nil {
		_ = os.RemoveAll(configDir)

		return nil, nil, fmt.Errorf("write gemini settings.json: %w", writeErr)
	}

	return func() error { return os.RemoveAll(configDir) },
		[]string{fmt.Sprintf("GEMINI_CLI_CONFIG_DIR=%s", configDir)},
		nil
}

var (
	_ harnesstype.Executor         = (*Executor)(nil)
	_ harnesstype.InputReceiver    = (*Executor)(nil)
	_ harnesstype.Resizable        = (*Executor)(nil)
	_ harnesstype.InterruptHandler = (*Executor)(nil)
	_ harnesstype.Refreshable      = (*Executor)(nil)
)
