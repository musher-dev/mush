package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
)

const (
	stateFileName = "update-check"
	checkInterval = 24 * time.Hour
)

// State holds cached update check results.
type State struct {
	LastCheckedAt  time.Time `json:"lastCheckedAt"`
	LatestVersion  string    `json:"latestVersion,omitempty"`
	CurrentVersion string    `json:"currentVersion,omitempty"`
	ReleaseURL     string    `json:"releaseURL,omitempty"`
}

// statePath returns the path to the state file.
func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mush", stateFileName), nil
}

// LoadState reads the state file. Returns zero-value State if the file doesn't exist.
func LoadState() (*State, error) {
	path, err := statePath()
	if err != nil {
		return &State{}, nil //nolint:nilerr // graceful: treat path failure as empty state
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path from controlled config directory
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupted state file; treat as empty
		return &State{}, nil //nolint:nilerr // graceful: corrupted state file treated as empty
	}

	return &s, nil
}

// SaveState writes the state file atomically.
func SaveState(s *State) error {
	path, err := statePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return mkdirErr
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	// Atomic write: unique temp file + rename.
	// Use CreateTemp to avoid clobbering from concurrent processes.
	// Try rename first (atomic on Unix). If it fails (e.g. Windows where dest exists),
	// fall back to remove + rename. Clean up temp file on failure.
	tmpFile, err := os.CreateTemp(dir, stateFileName+".*.tmp")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	if _, writeErr := tmpFile.Write(data); writeErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		// Fallback for Windows: remove dest then retry rename
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(tmp) // best-effort cleanup
			return removeErr
		}
		if retryErr := os.Rename(tmp, path); retryErr != nil {
			_ = os.Remove(tmp) // best-effort cleanup
			return retryErr
		}
	}
	return nil
}

// ShouldCheck returns true if enough time has passed since the last check.
func (s *State) ShouldCheck() bool {
	if s.LastCheckedAt.IsZero() {
		return true
	}
	return time.Since(s.LastCheckedAt) >= checkInterval
}

// HasUpdate returns true if the cached latest version is newer than current.
func (s *State) HasUpdate(currentVersion string) bool {
	if s.LatestVersion == "" || currentVersion == "" {
		return false
	}

	current, err := semver.NewVersion(currentVersion)
	if err != nil {
		return false
	}

	latest, err := semver.NewVersion(s.LatestVersion)
	if err != nil {
		return false
	}

	return latest.GreaterThan(current)
}
