// Package claude provides utilities for checking Claude Code CLI availability.
package claude

import (
	"os/exec"
)

// Available checks if Claude Code CLI is available.
func Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}
