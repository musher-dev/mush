package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstalledBundle records information about an installed bundle.
type InstalledBundle struct {
	Slug    string   `json:"slug"`
	Version string   `json:"version"`
	Harness string   `json:"harness"`
	Assets  []string `json:"assets"` // installed file paths (relative to workDir)

	Timestamp time.Time `json:"timestamp"`
}

const installedFileName = "installed.json"

// TrackInstall records a bundle installation in .mush/installed.json.
func TrackInstall(workDir string, bundle *InstalledBundle) error {
	installed, _ := LoadInstalled(workDir)

	// Replace existing entry for same slug+harness or append.
	found := false

	for i, b := range installed {
		if b.Slug == bundle.Slug && b.Harness == bundle.Harness {
			installed[i] = *bundle
			found = true

			break
		}
	}

	if !found {
		installed = append(installed, *bundle)
	}

	return saveInstalled(workDir, installed)
}

// LoadInstalled reads the list of installed bundles from .mush/installed.json.
func LoadInstalled(workDir string) ([]InstalledBundle, error) {
	path := filepath.Join(workDir, ".mush", installedFileName)

	data, err := os.ReadFile(path) //nolint:gosec // G304: path from known .mush directory
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read installed bundles: %w", err)
	}

	var installed []InstalledBundle
	if err := json.Unmarshal(data, &installed); err != nil {
		return nil, fmt.Errorf("parse installed bundles: %w", err)
	}

	return installed, nil
}

// FindInstalled looks up a specific installed bundle by slug and harness.
// Returns ErrNotInstalled if no matching bundle is found.
func FindInstalled(workDir, slug, harness string) (*InstalledBundle, error) {
	installed, err := LoadInstalled(workDir)
	if err != nil {
		return nil, err
	}

	for i := range installed {
		if installed[i].Slug == slug && installed[i].Harness == harness {
			return &installed[i], nil
		}
	}

	return nil, ErrNotInstalled
}

// ErrNotInstalled is returned when a bundle is not found in the installed list.
var ErrNotInstalled = errors.New("bundle not installed")

// Uninstall removes installed assets for a specific bundle slug and harness.
func Uninstall(workDir, slug, harness string) ([]string, error) {
	installed, err := LoadInstalled(workDir)
	if err != nil {
		return nil, err
	}

	target := -1

	for i, entry := range installed {
		if entry.Slug == slug && entry.Harness == harness {
			target = i
			break
		}
	}

	if target == -1 {
		return nil, nil
	}

	entry := installed[target]
	removed := make([]string, 0, len(entry.Assets))

	for _, relPath := range entry.Assets {
		absPath := filepath.Join(workDir, relPath)
		cleanWorkDir := filepath.Clean(workDir)
		cleanAbsPath := filepath.Clean(absPath)

		if !strings.HasPrefix(cleanAbsPath, cleanWorkDir+string(filepath.Separator)) && cleanAbsPath != cleanWorkDir {
			return nil, fmt.Errorf("refusing to remove path outside workdir: %s", relPath)
		}

		if err := os.Remove(cleanAbsPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove %s: %w", relPath, err)
		}

		removed = append(removed, relPath)
	}

	installed = append(installed[:target], installed[target+1:]...)
	if err := saveInstalled(workDir, installed); err != nil {
		return nil, err
	}

	return removed, nil
}

func saveInstalled(workDir string, installed []InstalledBundle) error {
	mushDir := filepath.Join(workDir, ".mush")
	if err := os.MkdirAll(mushDir, 0o755); err != nil { //nolint:gosec // G301: project dir
		return fmt.Errorf("create .mush directory: %w", err)
	}

	data, err := json.MarshalIndent(installed, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal installed bundles: %w", err)
	}

	dest := filepath.Join(mushDir, installedFileName)

	// Atomic write: temp file in same dir + rename.
	tmpFile, err := os.CreateTemp(mushDir, installedFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp installed file: %w", err)
	}

	tmp := tmpFile.Name()

	if _, writeErr := tmpFile.Write(data); writeErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("write temp installed file: %w", writeErr)
	}

	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp installed file: %w", closeErr)
	}

	if err := os.Rename(tmp, dest); err != nil {
		// Fallback for Windows: remove dest then retry rename.
		if removeErr := os.Remove(dest); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(tmp)
			return fmt.Errorf("remove existing installed file: %w", removeErr)
		}

		if retryErr := os.Rename(tmp, dest); retryErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("replace installed file: %w", retryErr)
		}
	}

	return nil
}
