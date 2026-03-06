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
	Namespace string   `json:"namespace"`
	Slug      string   `json:"slug"`
	Ref       string   `json:"ref"`
	Version   string   `json:"version"`
	Harness   string   `json:"harness"`
	Assets    []string `json:"assets"` // installed file paths (relative to workDir)

	Timestamp time.Time `json:"timestamp"`
}

const installedFileName = "installed.json"

// TrackInstall records a bundle installation in .mush/installed.json.
func TrackInstall(workDir string, bundle *InstalledBundle) error {
	if bundle.Namespace == "" || bundle.Slug == "" {
		return fmt.Errorf("installed bundle must include namespace and slug")
	}

	if bundle.Ref == "" {
		bundle.Ref = bundle.Namespace + "/" + bundle.Slug
	}

	installed, _ := LoadInstalled(workDir)

	// Replace existing entry for same ref+harness or append.
	found := false

	for i := range installed {
		if installed[i].Ref == bundle.Ref && installed[i].Harness == bundle.Harness {
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

	// Backfill legacy entries that lack namespace/ref fields.
	for i := range installed {
		if installed[i].Slug == "" {
			continue // skip truly empty entries
		}

		if installed[i].Namespace == "" {
			// Legacy entries stored slug as "namespace/slug"; split if possible.
			if parts := strings.SplitN(installed[i].Slug, "/", 2); len(parts) == 2 {
				installed[i].Namespace = parts[0]
				installed[i].Slug = parts[1]
			}
		}

		if installed[i].Ref == "" && installed[i].Namespace != "" {
			installed[i].Ref = installed[i].Namespace + "/" + installed[i].Slug
		}
	}

	return installed, nil
}

// FindInstalled looks up a specific installed bundle by ref and harness.
// Returns ErrNotInstalled if no matching bundle is found.
func FindInstalled(workDir string, ref Ref, harness string) (*InstalledBundle, error) {
	installed, err := LoadInstalled(workDir)
	if err != nil {
		return nil, err
	}

	baseRef := ref.Namespace + "/" + ref.Slug

	for i := range installed {
		if installed[i].Ref != baseRef || installed[i].Harness != harness {
			continue
		}

		if ref.Version != "" && installed[i].Version != ref.Version {
			continue
		}

		if installed[i].Namespace != ref.Namespace || installed[i].Slug != ref.Slug {
			continue
		}

		return &installed[i], nil
	}

	return nil, ErrNotInstalled
}

// ErrNotInstalled is returned when a bundle is not found in the installed list.
var ErrNotInstalled = errors.New("bundle not installed")

// Uninstall removes installed assets for a specific bundle reference and harness.
func Uninstall(workDir string, ref Ref, harness string) ([]string, error) {
	installed, err := LoadInstalled(workDir)
	if err != nil {
		return nil, err
	}

	target := -1
	baseRef := ref.Namespace + "/" + ref.Slug

	for i := range installed {
		if installed[i].Ref != baseRef || installed[i].Harness != harness {
			continue
		}

		if ref.Version != "" && installed[i].Version != ref.Version {
			continue
		}

		target = i

		break
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
