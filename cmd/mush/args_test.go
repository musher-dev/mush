package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

// TestAllRunnableCommandsHaveArgsValidator walks the entire command tree and
// fails if any runnable command (one with RunE or Run) is missing an Args
// validator. This prevents future commands from shipping without validators.
func TestAllRunnableCommandsHaveArgsValidator(t *testing.T) {
	root := newRootCmd()

	var missing []string

	for _, cmd := range collectAllCommands(root) {
		if !cmd.Runnable() {
			continue
		}

		if cmd.Args == nil {
			missing = append(missing, cmd.CommandPath())
		}
	}

	if len(missing) > 0 {
		t.Errorf("runnable commands missing Args validator:\n  %s\n\nAdd Args: noArgs (or another validator) to each command.",
			strings.Join(missing, "\n  "))
	}
}

// collectAllCommands returns every command in the tree (including root).
func collectAllCommands(root *cobra.Command) []*cobra.Command {
	var all []*cobra.Command

	var walk func(cmd *cobra.Command)

	walk = func(cmd *cobra.Command) {
		all = append(all, cmd)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}

	walk(root)

	return all
}

// TestUnknownFlagReturnsCLIError verifies that SetFlagErrorFunc wraps flag
// errors as CLIError with the correct code, message, and hint.
func TestUnknownFlagReturnsCLIError(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"version", "--bogus"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Errorf("exit code = %d, want %d (ExitUsage)", cliErr.Code, clierrors.ExitUsage)
	}

	if !strings.Contains(cliErr.Message, "unknown flag") {
		t.Errorf("message = %q, want to contain 'unknown flag'", cliErr.Message)
	}

	if !strings.Contains(cliErr.Hint, "--help") {
		t.Errorf("hint = %q, want to contain '--help'", cliErr.Hint)
	}

	if !strings.Contains(cliErr.Hint, "mush version") {
		t.Errorf("hint = %q, want to contain command path 'mush version'", cliErr.Hint)
	}
}

// TestNoArgsCommandRejectsExtraArgs verifies that commands with noArgs reject
// positional arguments with a clear message and hint.
func TestNoArgsCommandRejectsExtraArgs(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"version", "extra"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for extra argument, got nil")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Errorf("exit code = %d, want %d (ExitUsage)", cliErr.Code, clierrors.ExitUsage)
	}

	if !strings.Contains(cliErr.Message, "accepts no arguments") {
		t.Errorf("message = %q, want to contain 'accepts no arguments'", cliErr.Message)
	}

	if !strings.Contains(cliErr.Hint, "--help") {
		t.Errorf("hint = %q, want to contain '--help'", cliErr.Hint)
	}
}
