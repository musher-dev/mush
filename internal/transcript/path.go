package transcript

import (
	"os"
	"path/filepath"
)

// DefaultDir returns the default history directory.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mush", "history"), nil
}
