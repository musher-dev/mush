// Package codex provides utilities for checking OpenAI Codex CLI availability.
package codex

import (
	"os/exec"
)

// Available checks if the Codex CLI is available.
func Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}
