// Package errors provides structured CLI error types for Mush.
//
// CLIError wraps errors with user-facing messages, hints, and exit codes
// to provide consistent, actionable error output across all commands.
package errors

import (
	"errors"
	"fmt"
	"strings"
)

// Exit codes for CLI errors.
const (
	ExitSuccess   = 0  // Successful execution
	ExitGeneral   = 1  // General error
	ExitAuth      = 2  // Authentication error
	ExitNetwork   = 3  // Network/API error
	ExitConfig    = 4  // Configuration error
	ExitTimeout   = 5  // Execution timeout
	ExitExecution = 6  // Execution failure
	ExitUsage     = 64 // Command line usage error (BSD convention)
)

// CLIError represents a user-facing CLI error with actionable guidance.
type CLIError struct {
	// Message is the primary error message shown to the user.
	Message string

	// Hint provides actionable guidance on how to fix the error.
	Hint string

	// Cause is the underlying error, if any.
	Cause error

	// Code is the exit code for the CLI.
	Code int
}

// Error implements the error interface.
func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}

	return e.Message
}

// Unwrap returns the underlying cause for errors.Is/As support.
func (e *CLIError) Unwrap() error {
	return e.Cause
}

// New creates a new CLIError with the given message and exit code.
func New(code int, message string) *CLIError {
	return &CLIError{
		Message: message,
		Code:    code,
	}
}

// Wrap wraps an existing error with a CLIError.
func Wrap(code int, message string, cause error) *CLIError {
	return &CLIError{
		Message: message,
		Cause:   cause,
		Code:    code,
	}
}

// WithHint adds a hint to the error.
func (e *CLIError) WithHint(hint string) *CLIError {
	e.Hint = hint
	return e
}

// As is a convenience function for errors.As with CLIError.
func As(err error, target **CLIError) bool {
	return errors.As(err, target)
}

// --- Common error constructors ---

// NotAuthenticated returns an error indicating missing credentials.
func NotAuthenticated() *CLIError {
	return &CLIError{
		Message: "Not authenticated",
		Hint:    "Run 'mush auth login' to authenticate",
		Code:    ExitAuth,
	}
}

// AuthFailed returns an error for failed authentication.
func AuthFailed(cause error) *CLIError {
	return &CLIError{
		Message: "Authentication failed",
		Hint:    "Check your API key or run 'mush auth login'",
		Cause:   cause,
		Code:    ExitAuth,
	}
}

// CredentialsInvalid returns an error for invalid stored credentials.
func CredentialsInvalid(cause error) *CLIError {
	return &CLIError{
		Message: "Credentials invalid",
		Hint:    "Run 'mush auth login' to re-authenticate",
		Cause:   cause,
		Code:    ExitAuth,
	}
}

// CannotPrompt returns an error when interactive prompts are unavailable.
func CannotPrompt(envVar string) *CLIError {
	return &CLIError{
		Message: "Cannot prompt in non-interactive mode",
		Hint:    fmt.Sprintf("Set %s environment variable instead", envVar),
		Code:    ExitUsage,
	}
}

// HabitatNotFound returns an error for an unknown habitat.
func HabitatNotFound(name string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Habitat not found: %s", name),
		Hint:    "Run 'mush habitat list' to see available habitats",
		Code:    ExitConfig,
	}
}

// NoHabitats returns an error when no habitats exist in the workspace.
func NoHabitats() *CLIError {
	return &CLIError{
		Message: "No habitats found in workspace",
		Hint:    "Create a habitat in the console first",
		Code:    ExitConfig,
	}
}

// QueueNotFound returns an error for an unknown queue.
func QueueNotFound(name string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Queue not found: %s", name),
		Hint:    "Run 'mush link' and select a queue, or pass a valid --queue-id",
		Code:    ExitConfig,
	}
}

// NoQueuesForHabitat returns an error when a habitat has no eligible queues.
func NoQueuesForHabitat() *CLIError {
	return &CLIError{
		Message: "No active queues found for the selected habitat",
		Hint:    "Create or bind an active queue to this habitat, then retry",
		Code:    ExitConfig,
	}
}

// NoInstructionsForQueue returns an error when no active instruction exists for a queue.
func NoInstructionsForQueue(queueName, queueSlug string) *CLIError {
	label := queueName
	if label == "" {
		label = queueSlug
	}

	if queueName != "" && queueSlug != "" {
		label = fmt.Sprintf("%s (%s)", queueName, queueSlug)
	}

	if label == "" {
		label = "unknown"
	}

	return &CLIError{
		Message: fmt.Sprintf("No active instructions found for queue: %s", label),
		Hint:    "Create and activate an instruction for this queue in the console, then rerun 'mush link'",
		Code:    ExitConfig,
	}
}

// HabitatRequired returns an error when a habitat is required but not specified.
func HabitatRequired() *CLIError {
	return &CLIError{
		Message: "Habitat required",
		Hint:    "Run 'mush habitat list' to see available habitats",
		Code:    ExitConfig,
	}
}

// QueueRequired returns an error when a queue is required but not specified.
func QueueRequired() *CLIError {
	return &CLIError{
		Message: "Queue required",
		Hint:    "Use --queue-id to specify a queue, or run without --no-input to select interactively",
		Code:    ExitUsage,
	}
}

// APIKeyEmpty returns an error when the API key is empty.
func APIKeyEmpty() *CLIError {
	return &CLIError{
		Message: "API key cannot be empty",
		Hint:    "Enter a valid API key or set MUSHER_API_KEY environment variable",
		Code:    ExitAuth,
	}
}

// ConfigFailed returns an error for configuration save failures.
func ConfigFailed(operation string, cause error) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Failed to %s", operation),
		Hint:    "Check file permissions for your Mush config directory or run 'mush doctor'",
		Cause:   cause,
		Code:    ExitConfig,
	}
}

// JobNotFound returns an error for an unknown job.
func JobNotFound(jobID string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Job not found: %s", jobID),
		Hint:    "The job may have been completed, canceled, or the ID is incorrect",
		Code:    ExitGeneral,
	}
}

// LinkRegistrationFailed returns an error when link registration fails.
func LinkRegistrationFailed(cause error) *CLIError {
	return &CLIError{
		Message: "Failed to register link",
		Hint:    "Check your network connection and API credentials",
		Cause:   cause,
		Code:    ExitNetwork,
	}
}

// ExecutionTimedOut returns an error for execution timeout with context.
func ExecutionTimedOut(timeout string, lastTools []string) *CLIError {
	hint := "Increase timeout or simplify the job"

	if len(lastTools) > 0 {
		// Show last tool for context
		lastTool := lastTools[len(lastTools)-1]
		hint += fmt.Sprintf(". Last activity: %s", lastTool)
	}

	return &CLIError{
		Message: fmt.Sprintf("Execution timed out after %s", timeout),
		Hint:    hint,
		Code:    ExitTimeout,
	}
}

// ClaudeExecutionFailed returns an error for Claude execution failures.
// It detects common error patterns and provides specific hints.
func ClaudeExecutionFailed(exitCode int, stderr string) *CLIError {
	msg := "Claude execution failed"
	hint := ""

	// Detect common error patterns
	switch {
	case containsAny(stderr, "rate limit", "rate_limit", "429"):
		msg = "Claude API rate limit exceeded"
		hint = "Wait a moment and try again, or check your API usage limits"
	case containsAny(stderr, "authentication", "unauthorized", "401", "invalid_api_key"):
		msg = "Claude API authentication failed"
		hint = "Check your ANTHROPIC_API_KEY environment variable"
	case containsAny(stderr, "context length", "context_length", "max_tokens"):
		msg = "Claude context length exceeded"
		hint = "Simplify the job or break it into smaller parts"
	case containsAny(stderr, "overloaded", "503", "service unavailable"):
		msg = "Claude API is temporarily overloaded"
		hint = "Wait a moment and try again"
	case containsAny(stderr, "connection", "network", "timeout"):
		msg = "Network error connecting to Claude API"
		hint = "Check your network connection"
	case exitCode == 1 && stderr == "":
		hint = "Run with --log-level=debug for more details"
	default:
		if stderr != "" {
			// Truncate long error messages
			if len(stderr) > 200 {
				stderr = stderr[:200] + "..."
			}

			hint = stderr
		}
	}

	return &CLIError{
		Message: msg,
		Hint:    hint,
		Code:    ExitExecution,
	}
}

// ClaudeSignalKilled returns an error for interrupted execution.
func ClaudeSignalKilled() *CLIError {
	return &CLIError{
		Message: "Claude execution was interrupted",
		Hint:    "The process was terminated by a signal",
		Code:    ExitExecution,
	}
}

// ClaudeNotFound returns an error when Claude CLI is not available.
func ClaudeNotFound() *CLIError {
	return &CLIError{
		Message: "Claude CLI not found",
		Hint:    "Install Claude Code CLI: https://docs.anthropic.com/en/docs/claude-code",
		Code:    ExitConfig,
	}
}

// InvalidHarnessType returns an error for an unsupported harness type.
func InvalidHarnessType(harnessType string, supported []string) *CLIError {
	hint := "No harness types registered"
	if len(supported) > 0 {
		hint = fmt.Sprintf("Supported harness types: %s", strings.Join(supported, ", "))
	}

	return &CLIError{
		Message: fmt.Sprintf("Invalid harness type: %s", harnessType),
		Hint:    hint,
		Code:    ExitUsage,
	}
}

// HarnessNotAvailable returns an error when a harness runtime is not installed.
func HarnessNotAvailable(harnessType string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("%s CLI not found", harnessType),
		Hint:    fmt.Sprintf("Install the %s CLI to use this harness type", harnessType),
		Code:    ExitConfig,
	}
}

// CodexNotFound returns an error when Codex CLI is not available.
func CodexNotFound() *CLIError {
	return &CLIError{
		Message: "Codex CLI not found",
		Hint:    "Install OpenAI Codex CLI: https://github.com/openai/codex",
		Code:    ExitConfig,
	}
}

// BundleNotFound returns an error for an unknown bundle.
func BundleNotFound(slug string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Bundle not found: %s", slug),
		Hint:    "Check the bundle slug or verify it exists in your workspace",
		Code:    ExitGeneral,
	}
}

// BundleVersionNotFound returns an error for an unknown bundle version.
func BundleVersionNotFound(slug, version string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Bundle version not found: %s:%s", slug, version),
		Hint:    "Check the version number or omit it to use the latest version",
		Code:    ExitGeneral,
	}
}

// BundleIntegrityFailed returns an error when bundle asset verification fails.
func BundleIntegrityFailed(path string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Bundle integrity check failed for: %s", path),
		Hint:    "The downloaded asset does not match its expected checksum. Try again or contact support",
		Code:    ExitGeneral,
	}
}

// PathTraversalBlocked returns an error when a path traversal attempt is detected.
func PathTraversalBlocked(path string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Path traversal blocked: %s", path),
		Hint:    "Bundle assets must not reference paths outside the target directory",
		Code:    ExitGeneral,
	}
}

// InstallConflict returns an error when a bundle asset conflicts with existing files.
func InstallConflict(path string) *CLIError {
	return &CLIError{
		Message: fmt.Sprintf("Install conflict: file already exists at %s", path),
		Hint:    "Use --force to overwrite existing files",
		Code:    ExitGeneral,
	}
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}

	return false
}
