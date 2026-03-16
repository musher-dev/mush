package executil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// LookPath resolves a binary from PATH and rejects current-directory execution.
func LookPath(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("resolve executable: empty command name")
	}

	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}

	if errors.Is(err, exec.ErrDot) {
		return "", fmt.Errorf("resolve %q: refusing to execute from current directory: %w", name, err)
	}

	return "", fmt.Errorf("resolve %q: %w", name, err)
}

// CommandContext creates an exec.Cmd from a PATH-resolved binary.
func CommandContext(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	path, err := LookPath(name)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path is resolved via LookPath and ErrDot is rejected

	return cmd, nil
}

// AbsoluteCommandContext creates an exec.Cmd from an absolute executable path.
func AbsoluteCommandContext(ctx context.Context, executablePath string, args ...string) (*exec.Cmd, error) {
	if !filepath.IsAbs(executablePath) {
		return nil, fmt.Errorf("resolve executable path: expected absolute path, got %q", executablePath)
	}

	cleanPath := filepath.Clean(executablePath)
	cmd := exec.CommandContext(ctx, cleanPath, args...) //nolint:gosec // caller provides an absolute executable path

	return cmd, nil
}
