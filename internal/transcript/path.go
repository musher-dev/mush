package transcript

import (
	"fmt"

	"github.com/musher-dev/mush/internal/paths"
)

// DefaultDir returns the default history directory.
func DefaultDir() (string, error) {
	historyDir, err := paths.HistoryDir()
	if err != nil {
		return "", fmt.Errorf("resolve transcript history directory: %w", err)
	}

	return historyDir, nil
}
