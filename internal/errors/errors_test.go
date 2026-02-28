package errors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/testutil"
)

func TestExecutionTimedOut(t *testing.T) {
	tests := []struct {
		name      string
		timeout   string
		lastTools []string
		wantMsg   string
		wantHint  string
	}{
		{
			name:      "no tools",
			timeout:   "1m0s",
			lastTools: nil,
			wantMsg:   "Execution timed out after 1m0s",
			wantHint:  "Increase timeout or simplify the job",
		},
		{
			name:      "with last tool",
			timeout:   "5m0s",
			lastTools: []string{"Read", "Bash", "Edit"},
			wantMsg:   "Execution timed out after 5m0s",
			wantHint:  "Last activity: Edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecutionTimedOut(tt.timeout, tt.lastTools)

			if !strings.Contains(err.Message, tt.wantMsg) {
				t.Errorf("message = %q, want to contain %q", err.Message, tt.wantMsg)
			}

			if !strings.Contains(err.Hint, tt.wantHint) {
				t.Errorf("hint = %q, want to contain %q", err.Hint, tt.wantHint)
			}

			if err.Code != ExitTimeout {
				t.Errorf("code = %d, want %d", err.Code, ExitTimeout)
			}
		})
	}
}

func TestClaudeExecutionFailed(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		wantMsg  string
		wantHint string
	}{
		{
			name:     "rate limit",
			exitCode: 1,
			stderr:   "Error: rate limit exceeded",
			wantMsg:  "rate limit",
			wantHint: "Wait a moment",
		},
		{
			name:     "authentication",
			exitCode: 1,
			stderr:   "Error: unauthorized access",
			wantMsg:  "authentication",
			wantHint: "ANTHROPIC_API_KEY",
		},
		{
			name:     "context length",
			exitCode: 1,
			stderr:   "Error: context length exceeded max_tokens",
			wantMsg:  "context length",
			wantHint: "Simplify",
		},
		{
			name:     "overloaded",
			exitCode: 1,
			stderr:   "Error: 503 service unavailable",
			wantMsg:  "overloaded",
			wantHint: "Wait a moment",
		},
		{
			name:     "empty stderr exit 1",
			exitCode: 1,
			stderr:   "",
			wantMsg:  "failed",
			wantHint: "--log-level=debug",
		},
		{
			name:     "generic error",
			exitCode: 1,
			stderr:   "Some unknown error occurred",
			wantMsg:  "failed",
			wantHint: "Some unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClaudeExecutionFailed(tt.exitCode, tt.stderr)

			if !strings.Contains(strings.ToLower(err.Message), strings.ToLower(tt.wantMsg)) {
				t.Errorf("message = %q, want to contain %q", err.Message, tt.wantMsg)
			}

			if !strings.Contains(err.Hint, tt.wantHint) {
				t.Errorf("hint = %q, want to contain %q", err.Hint, tt.wantHint)
			}

			if err.Code != ExitExecution {
				t.Errorf("code = %d, want %d", err.Code, ExitExecution)
			}
		})
	}
}

func TestClaudeSignalKilled(t *testing.T) {
	err := ClaudeSignalKilled()

	if !strings.Contains(err.Message, "interrupted") {
		t.Errorf("message = %q, want to contain 'interrupted'", err.Message)
	}

	if err.Code != ExitExecution {
		t.Errorf("code = %d, want %d", err.Code, ExitExecution)
	}
}

func TestClaudeNotFound(t *testing.T) {
	err := ClaudeNotFound()

	if !strings.Contains(err.Message, "not found") {
		t.Errorf("message = %q, want to contain 'not found'", err.Message)
	}

	if !strings.Contains(err.Hint, "Install") {
		t.Errorf("hint = %q, want to contain 'Install'", err.Hint)
	}

	if err.Code != ExitConfig {
		t.Errorf("code = %d, want %d", err.Code, ExitConfig)
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s          string
		substrings []string
		want       bool
	}{
		{"rate limit exceeded", []string{"rate limit"}, true},
		{"RATE LIMIT exceeded", []string{"rate limit"}, true},
		{"some error", []string{"rate limit", "auth"}, false},
		{"authentication failed", []string{"rate limit", "auth"}, true},
		{"", []string{"test"}, false},
	}

	for _, tt := range tests {
		result := containsAny(tt.s, tt.substrings...)
		if result != tt.want {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrings, result, tt.want)
		}
	}
}

// TestAllErrorsHaveHints verifies that all error constructors provide actionable hints.
func TestAllErrorsHaveHints(t *testing.T) {
	tests := []struct {
		name string
		err  *CLIError
	}{
		{"NotAuthenticated", NotAuthenticated()},
		{"AuthFailed", AuthFailed(nil)},
		{"CredentialsInvalid", CredentialsInvalid(nil)},
		{"CannotPrompt", CannotPrompt("TEST_VAR")},
		{"HabitatNotFound", HabitatNotFound("test")},
		{"NoHabitats", NoHabitats()},
		{"QueueNotFound", QueueNotFound("queue-123")},
		{"NoQueuesForHabitat", NoQueuesForHabitat()},
		{"HabitatRequired", HabitatRequired()},
		{"APIKeyEmpty", APIKeyEmpty()},
		{"ConfigFailed", ConfigFailed("test operation", nil)},
		{"JobNotFound", JobNotFound("job-123")},
		{"WorkerRegistrationFailed", WorkerRegistrationFailed(nil)},
		{"ExecutionTimedOut", ExecutionTimedOut("1m", nil)},
		{"ClaudeExecutionFailed", ClaudeExecutionFailed(1, "error message")},
		{"ClaudeSignalKilled", ClaudeSignalKilled()},
		{"ClaudeNotFound", ClaudeNotFound()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Hint == "" {
				t.Errorf("%s() should have a hint, got empty string", tt.name)
			}

			if tt.err.Message == "" {
				t.Errorf("%s() should have a message, got empty string", tt.name)
			}
		})
	}
}

func TestCLIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *CLIError
		want string
	}{
		{
			name: "message only",
			err:  &CLIError{Message: "test error"},
			want: "test error",
		},
		{
			name: "message with cause",
			err:  &CLIError{Message: "test error", Cause: New(1, "underlying")},
			want: "test error: underlying",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	cause := New(1, "cause")
	err := &CLIError{Message: "wrapper", Cause: cause}

	if got := err.Unwrap(); got != cause { //nolint:errorlint // testing identity
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestWithHint(t *testing.T) {
	err := New(1, "test").WithHint("do this")

	if err.Hint != "do this" {
		t.Errorf("WithHint() hint = %q, want %q", err.Hint, "do this")
	}
}

func TestWrap(t *testing.T) {
	cause := New(1, "cause")
	err := Wrap(ExitNetwork, "wrapped", cause)

	if err.Code != ExitNetwork {
		t.Errorf("Wrap() code = %d, want %d", err.Code, ExitNetwork)
	}

	if err.Cause != cause { //nolint:errorlint // testing struct field identity
		t.Errorf("Wrap() cause = %v, want %v", err.Cause, cause)
	}
}

// formatCLIError produces a deterministic string representation of a CLIError for golden file comparison.
func formatCLIError(err *CLIError) string {
	return fmt.Sprintf("Message: %s\nHint: %s\nCode: %d\n", err.Message, err.Hint, err.Code)
}

func TestErrorMessages_Golden(t *testing.T) {
	tests := []struct {
		name string
		err  *CLIError
	}{
		{"NotAuthenticated", NotAuthenticated()},
		{"AuthFailed", AuthFailed(nil)},
		{"CredentialsInvalid", CredentialsInvalid(nil)},
		{"CannotPrompt", CannotPrompt("MUSH_API_KEY")},
		{"HabitatNotFound", HabitatNotFound("prod-habitat")},
		{"NoHabitats", NoHabitats()},
		{"QueueNotFound", QueueNotFound("queue-123")},
		{"NoQueuesForHabitat", NoQueuesForHabitat()},
		{"NoInstructionsForQueue", NoInstructionsForQueue("My Queue", "my-queue")},
		{"HabitatRequired", HabitatRequired()},
		{"QueueRequired", QueueRequired()},
		{"APIKeyEmpty", APIKeyEmpty()},
		{"ConfigFailed", ConfigFailed("store credentials", nil)},
		{"JobNotFound", JobNotFound("job-abc-123")},
		{"WorkerRegistrationFailed", WorkerRegistrationFailed(nil)},
		{"ExecutionTimedOut_NoTools", ExecutionTimedOut("5m0s", nil)},
		{"ExecutionTimedOut_WithTools", ExecutionTimedOut("5m0s", []string{"Read", "Bash", "Edit"})},
		{"ClaudeExecutionFailed_RateLimit", ClaudeExecutionFailed(1, "rate limit exceeded")},
		{"ClaudeExecutionFailed_Auth", ClaudeExecutionFailed(1, "unauthorized access")},
		{"ClaudeExecutionFailed_Generic", ClaudeExecutionFailed(1, "something broke")},
		{"ClaudeSignalKilled", ClaudeSignalKilled()},
		{"ClaudeNotFound", ClaudeNotFound()},
		{"CodexNotFound", CodexNotFound()},
		{"InvalidHarnessType", InvalidHarnessType("unknown", []string{"claude", "bash"})},
		{"HarnessNotAvailable", HarnessNotAvailable("claude")},
		{"BundleNotFound", BundleNotFound("my-bundle")},
		{"BundleVersionNotFound", BundleVersionNotFound("my-bundle", "1.0.0")},
		{"BundleIntegrityFailed", BundleIntegrityFailed("/tmp/asset.tar.gz")},
		{"PathTraversalBlocked", PathTraversalBlocked("../../etc/passwd")},
		{"InstallConflict", InstallConflict(".claude/skills/run.md")},
	}

	var sb strings.Builder
	for _, tt := range tests {
		fmt.Fprintf(&sb, "--- %s ---\n", tt.name)
		sb.WriteString(formatCLIError(tt.err))
		sb.WriteString("\n")
	}

	testutil.AssertGolden(t, sb.String(), "error_messages.golden")
}
