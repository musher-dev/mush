//go:build unix

package harnesstype

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/client"
)

// GetPromptFromJob extracts the prompt from a job's data and execution config.
func GetPromptFromJob(job *client.Job) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}

	if job.Execution == nil {
		return "", fmt.Errorf("missing execution config for job")
	}

	if rendered := job.GetRenderedInstruction(); rendered != "" {
		return rendered, nil
	}

	if job.ExecutionError != "" {
		return "", fmt.Errorf("server execution error: %s", job.ExecutionError)
	}

	return "", fmt.Errorf("missing execution.renderedInstruction for job")
}

// HandleOneShotRunError converts a one-shot executor run error into an *ExecError,
// handling context cancellation, deadline exceeded, and exit-code extraction.
func HandleOneShotRunError(ctx context.Context, runErr error, rawOutput, name string) *ExecError {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return &ExecError{Reason: "timeout", Message: fmt.Sprintf("%s execution timed out", name), Retry: true}
		}

		return &ExecError{
			Reason:  "execution_error",
			Message: fmt.Sprintf("%s execution canceled: %v", name, ctxErr),
			Retry:   true,
		}
	}

	exitCode := 1

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	msg := fmt.Sprintf("%s exited with code %d", name, exitCode)

	cleanOutput := ansi.Strip(strings.TrimSpace(rawOutput))
	if cleanOutput != "" {
		msg = fmt.Sprintf("%s: %s", msg, cleanOutput)
	}

	return &ExecError{
		Reason:  "execution_error",
		Message: msg,
		Retry:   true,
	}
}
