package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/musher-dev/mush/internal/client"
)

// CachedBundle is a locally cached bundle version.
type CachedBundle struct {
	Workspace  string
	Slug       string
	Version    string
	AssetCount int
}

// ListCached returns all cached bundle versions.
func ListCached() ([]CachedBundle, error) {
	root := CacheDir()

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read cache root: %w", err)
	}

	var out []CachedBundle

	for _, workspaceEntry := range entries {
		if !workspaceEntry.IsDir() {
			continue
		}

		wsPath := filepath.Join(root, workspaceEntry.Name())

		slugDirs, err := os.ReadDir(wsPath)
		if err != nil {
			continue
		}

		for _, slugDir := range slugDirs {
			if !slugDir.IsDir() {
				continue
			}

			slugPath := filepath.Join(wsPath, slugDir.Name())

			versionDirs, err := os.ReadDir(slugPath)
			if err != nil {
				continue
			}

			for _, versionDir := range versionDirs {
				if !versionDir.IsDir() {
					continue
				}

				versionPath := filepath.Join(slugPath, versionDir.Name())
				manifestPath := filepath.Join(versionPath, "manifest.json")

				data, err := os.ReadFile(manifestPath) //nolint:gosec // G304: controlled cache path
				if err != nil {
					continue
				}

				var manifest client.BundleResolveResponse
				if err := json.Unmarshal(data, &manifest); err != nil {
					continue
				}

				out = append(out, CachedBundle{
					Workspace:  workspaceEntry.Name(),
					Slug:       slugDir.Name(),
					Version:    versionDir.Name(),
					AssetCount: len(manifest.Manifest.Layers),
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Workspace != out[j].Workspace {
			return out[i].Workspace < out[j].Workspace
		}

		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}

		return out[i].Version < out[j].Version
	})

	return out, nil
}
