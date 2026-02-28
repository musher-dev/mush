//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
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

// CodexExecutor runs jobs via OpenAI Codex CLI.
// Each job runs in its own process — there is no persistent runtime.
type CodexExecutor struct {
	opts SetupOptions

	mu         sync.Mutex
	cmd        *exec.Cmd
	ptmx       *os.File
	pgid       int
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
	ptmx := e.ptmx
	e.mu.Unlock()

	if ptmx == nil {
		return 0, nil
	}

	n, err := ptmx.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to codex pty: %w", err)
	}

	return n, nil
}

func (e *CodexExecutor) startInteractive(ctx context.Context, opts *SetupOptions) error {
	spec, _ := GetProvider("codex")

	var args []string
	if !opts.BundleLoadMode {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if opts.BundleDir != "" {
		// --add-dir requires at least workspace-write sandbox mode;
		// without this, codex defaults to read-only and ignores --add-dir.
		args = append(args, "--sandbox", "workspace-write")
	}

	if opts.BundleDir != "" && spec != nil && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, opts.BundleDir)
	}

	cmd := exec.CommandContext(ctx, "codex", args...) //nolint:gosec // G204: args from controlled input

	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "FORCE_COLOR=1")

	// NOTE: cmd.Stdin/Stdout/Stderr must remain nil here.
	// creack/pty.StartWithSize assigns the PTY tty to all three;
	// pre-setting Stdin to a non-tty would break Setctty (fd 0 must be the tty).
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(opts.TermHeight),
		Cols: uint16(opts.TermWidth),
	})
	if err != nil {
		return fmt.Errorf("start codex interactive session: %w", err)
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

	// Read PTY output and forward to terminal writer.
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

// Teardown stops the interactive codex process when running in bundle mode.
func (e *CodexExecutor) Teardown() {
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

	if ptmx != nil {
		_ = ptmx.Close()
	}

	if cmd == nil || cmd.Process == nil {
		return
	}

	sendSignal(cmd.Process.Pid, pgid, syscall.SIGTERM)

	select {
	case <-waitDoneCh:
	case <-time.After(2 * time.Second):
		sendSignal(cmd.Process.Pid, pgid, syscall.SIGKILL)

		if waitDoneCh != nil {
			select {
			case <-waitDoneCh:
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// Ensure CodexExecutor satisfies the required interfaces.
var (
	_ Executor      = (*CodexExecutor)(nil)
	_ InputReceiver = (*CodexExecutor)(nil)
)
