//go:build unix

package harness

import (
	"context"
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
)

// CodexExecutor runs jobs via OpenAI Codex CLI.
// Each job runs in its own process — there is no persistent runtime.
type CodexExecutor struct {
	opts SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	waitDoneCh chan struct{}
}

func init() {
	Register(Info{
		Name:      "codex",
		Available: AvailableFunc("codex"),
		New:       func() Executor { return &CodexExecutor{} },
		MCPSpec: &MCPSpec{
			Def:         mustGetProvider("codex").MCP,
			BuildConfig: BuildTOMLMCPConfig,
		},
	})
}

// Setup stores options. Codex has no persistent process.
func (e *CodexExecutor) Setup(ctx context.Context, opts *SetupOptions) error {
	e.opts = *opts

	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex CLI not found in PATH")
	}

	// Bundle mode is an interactive, long-lived Codex session.
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

// Execute runs a codex command and returns the result.
func (e *CodexExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	if e.opts.BundleDir != "" {
		return nil, &ExecError{
			Reason:  "execution_error",
			Message: "codex interactive bundle mode does not support queued job execution",
		}
	}

	prompt, err := getPromptFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "prompt_error", Message: err.Error()}
	}

	// Create a temp file for codex output.
	outputFile, err := os.CreateTemp("", "mush-codex-output-*.txt")
	if err != nil {
		return nil, &ExecError{Reason: "execution_error", Message: fmt.Sprintf("failed to create output file: %v", err)}
	}

	outputPath := outputFile.Name()
	_ = outputFile.Close()

	defer func() {
		_ = os.Remove(outputPath)
	}()

	// Build codex command.
	args := []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}

	if job.Execution != nil && job.Execution.WorkingDirectory != "" {
		args = append(args, "-C", job.Execution.WorkingDirectory)
	}

	args = append(args, "-o", outputPath, prompt)

	cmd := exec.CommandContext(ctx, "codex", args...) //nolint:gosec // G204: command originates from trusted job execution payload

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

	// Pipe output to terminal.
	if e.opts.TermWriter != nil {
		cmd.Stdout = e.opts.TermWriter
		cmd.Stderr = e.opts.TermWriter
	}

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, &ExecError{Reason: "timeout", Message: "codex execution timed out", Retry: true}
			}

			return nil, &ExecError{Reason: "execution_error", Message: fmt.Sprintf("codex execution canceled: %v", ctxErr), Retry: true}
		}

		exitCode := 1

		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		return nil, &ExecError{
			Reason:  "codex_error",
			Message: fmt.Sprintf("codex exited with code %d: %v", exitCode, runErr),
			Retry:   true,
		}
	}

	// Read output from the output file.
	outputData, readErr := os.ReadFile(outputPath) //nolint:gosec // G304: path created by us via CreateTemp

	output := ""
	if readErr == nil {
		output = ansi.Strip(strings.TrimSpace(string(outputData)))
	}

	return &ExecResult{
		OutputData: map[string]any{
			"success":    true,
			"output":     output,
			"durationMs": int(duration / time.Millisecond),
		},
	}, nil
}

// Reset is a no-op for codex (each job is a separate process).
func (e *CodexExecutor) Reset(_ context.Context) error {
	return nil
}

// WriteInput forwards terminal input to the interactive Codex process.
func (e *CodexExecutor) WriteInput(p []byte) (int, error) {
	e.mu.Lock()
	stdin := e.stdin
	e.mu.Unlock()

	if stdin == nil {
		return 0, nil
	}

	n, err := stdin.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to codex stdin: %w", err)
	}

	return n, nil
}

func (e *CodexExecutor) startInteractive(ctx context.Context, opts *SetupOptions) error {
	spec, _ := GetProvider("codex")

	args := []string{
		"--dangerously-bypass-approvals-and-sandbox",
		"--skip-git-repo-check",
	}

	if opts.BundleDir != "" && spec != nil && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, opts.BundleDir)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("codex stdin pipe: %w", err)
	}

	var stdout io.Writer = os.Stdout
	if opts.TermWriter != nil {
		stdout = opts.TermWriter
	}

	if opts.OnOutput != nil {
		stdout = io.MultiWriter(stdout, outputCallbackWriter{fn: opts.OnOutput})
	}

	cmd.Stdout = stdout
	cmd.Stderr = stdout

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start codex interactive session: %w", err)
	}

	e.mu.Lock()
	e.cmd = cmd
	e.stdin = stdin
	e.waitDoneCh = make(chan struct{})
	waitDoneCh := e.waitDoneCh
	e.mu.Unlock()

	go func() {
		_ = cmd.Wait()

		close(waitDoneCh)

		if opts.OnExit != nil {
			opts.OnExit()
		}
	}()

	return nil
}

// Teardown stops the interactive codex process when running in bundle mode.
func (e *CodexExecutor) Teardown() {
	e.mu.Lock()
	cmd := e.cmd
	stdin := e.stdin
	waitDoneCh := e.waitDoneCh
	e.cmd = nil
	e.stdin = nil
	e.waitDoneCh = nil
	e.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}

	if cmd == nil || cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(os.Interrupt)

	select {
	case <-waitDoneCh:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()

		if waitDoneCh != nil {
			select {
			case <-waitDoneCh:
			case <-time.After(2 * time.Second):
			}
		}
	}
}

type outputCallbackWriter struct {
	fn func([]byte)
}

func (w outputCallbackWriter) Write(p []byte) (int, error) {
	if w.fn != nil {
		w.fn(p)
	}

	return len(p), nil
}

// Ensure CodexExecutor satisfies the required interfaces.
var (
	_ Executor      = (*CodexExecutor)(nil)
	_ InputReceiver = (*CodexExecutor)(nil)
)
