//go:build unix

package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
)

// BashExecutor runs jobs by executing bash commands in a subprocess.
// Each job runs in its own process — there is no persistent runtime.
type BashExecutor struct {
	opts SetupOptions
}

func init() {
	Register(Info{
		Name:      "bash",
		Available: func() bool { return true }, // checked at execution time
		New:       func() Executor { return &BashExecutor{} },
	})
}

// Setup stores options. Bash has no persistent process.
func (e *BashExecutor) Setup(_ context.Context, opts *SetupOptions) error {
	e.opts = *opts

	if opts.OnReady != nil {
		opts.OnReady()
	}

	return nil
}

// Execute runs a bash command and returns the result.
func (e *BashExecutor) Execute(ctx context.Context, job *client.Job) (*ExecResult, error) {
	command, err := getBashCommandFromJob(job)
	if err != nil {
		return nil, &ExecError{Reason: "command_error", Message: err.Error()}
	}

	outputData, execErr := e.executeBashJob(ctx, job, command)
	if execErr != nil {
		reason := "bash_error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}

		return nil, &ExecError{Reason: reason, Message: execErr.Error(), Retry: true}
	}

	outputData["success"] = true

	return &ExecResult{OutputData: outputData}, nil
}

// Reset is a no-op for bash (each job is a separate process).
func (e *BashExecutor) Reset(_ context.Context) error {
	return nil
}

// Teardown is a no-op for bash.
func (e *BashExecutor) Teardown() {}

func (e *BashExecutor) executeBashJob(ctx context.Context, job *client.Job, command string) (map[string]any, error) {
	if _, err := exec.LookPath("bash"); err != nil {
		return nil, fmt.Errorf("bash not found in PATH")
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command) //nolint:gosec // G204: command originates from trusted job execution payload

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

	var stdoutBuf, stderrBuf bytes.Buffer

	var stdout, stderr io.Writer

	stdout = &stdoutBuf
	stderr = &stderrBuf

	if e.opts.TermWriter != nil {
		stdout = io.MultiWriter(e.opts.TermWriter, &stdoutBuf)
		stderr = io.MultiWriter(e.opts.TermWriter, &stderrBuf)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	startedAt := time.Now()
	err := cmd.Run()
	duration := time.Since(startedAt)

	exitCode := 0

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("bash execution timed out: %w", ctxErr)
			}

			return nil, fmt.Errorf("bash execution canceled: %w", ctxErr)
		}

		exitCode = 1

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = err.Error()
		}

		return nil, fmt.Errorf("bash exited with code %d: %s", exitCode, msg)
	}

	return map[string]any{
		"output":     ansi.Strip(strings.TrimSpace(stdoutBuf.String())),
		"stdout":     stdoutBuf.String(),
		"stderr":     stderrBuf.String(),
		"exitCode":   exitCode,
		"durationMs": int(duration / time.Millisecond),
	}, nil
}

// getBashCommandFromJob extracts the bash command from a job.
func getBashCommandFromJob(job *client.Job) (string, error) {
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
		if cmd, ok := job.InputData["command"].(string); ok && cmd != "" {
			return cmd, nil
		}

		if script, ok := job.InputData["script"].(string); ok && script != "" {
			return script, nil
		}
	}

	return "", fmt.Errorf("no bash command found for job")
}

// Ensure BashExecutor satisfies the Executor interface.
var _ Executor = (*BashExecutor)(nil)
