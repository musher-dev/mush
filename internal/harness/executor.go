package harness

import (
	"context"
	"io"

	"github.com/musher-dev/mush/internal/client"
)

// Executor is the interface each harness type implements.
// Lifecycle: Setup → (Execute → Reset)* → Teardown.
type Executor interface {
	// Setup initializes the executor (e.g., starts a PTY process).
	Setup(ctx context.Context, opts *SetupOptions) error

	// Execute runs a job and returns the result.
	Execute(ctx context.Context, job *client.Job) (*ExecResult, error)

	// Reset prepares the executor for the next job (e.g., /clear).
	Reset(ctx context.Context) error

	// Teardown releases all resources.
	Teardown()
}

// Resizable is an optional interface for executors that can resize their terminal.
type Resizable interface {
	Resize(rows, cols int)
}

// InputReceiver is an optional interface for executors that accept stdin input.
type InputReceiver interface {
	WriteInput(p []byte) (int, error)
}

// Refreshable is an optional interface for executors that support runtime config refresh.
type Refreshable interface {
	NeedsRefresh(cfg *client.RunnerConfigResponse) bool
	ApplyRefresh(ctx context.Context, cfg *client.RunnerConfigResponse) error
}

// SetupOptions contains the configuration for executor setup.
type SetupOptions struct {
	// TermWriter is the writer for terminal output.
	TermWriter io.Writer

	// TermWidth and TermHeight are the terminal dimensions.
	TermWidth  int
	TermHeight int

	// SignalDir is the directory for signal files (used by Claude stop hook).
	SignalDir string

	// RunnerConfig is the initial runtime configuration.
	RunnerConfig *client.RunnerConfigResponse

	// BundleDir is the temp directory with harness-native bundle assets.
	BundleDir string

	// OnReady is called when the executor is ready to accept jobs.
	OnReady func()

	// OnOutput is called with output chunks for transcript/capture.
	OnOutput func(p []byte)

	// OnExit is called when a long-running interactive executor exits.
	OnExit func()
}

// ExecResult holds the result of a job execution.
type ExecResult struct {
	// OutputData is the structured output to report to the API.
	OutputData map[string]any
}

// ExecError represents a structured execution error.
type ExecError struct {
	// Reason is the error classification (e.g., "timeout", "execution_error").
	Reason string

	// Message is the human-readable error description.
	Message string

	// Retry indicates whether the job should be retried.
	Retry bool
}

func (e *ExecError) Error() string {
	return e.Message
}
