package transcript

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDir returns the default history directory.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	return filepath.Join(home, ".config", "mush", "history"), nil
}
