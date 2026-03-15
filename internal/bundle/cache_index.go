package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/safeio"
)

// CachedBundle is a locally cached bundle version.
type CachedBundle struct {
	Namespace  string
	Slug       string
	Version    string
	AssetCount int
	ModTime    time.Time // modification time of the cache directory
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

	for _, nsEntry := range entries {
		if !nsEntry.IsDir() {
			continue
		}

		nsPath := filepath.Join(root, nsEntry.Name())

		slugDirs, err := os.ReadDir(nsPath)
		if err != nil {
			continue
		}

		for _, slugDir := range slugDirs {
			if !slugDir.IsDir() {
				continue
			}

			slugPath := filepath.Join(nsPath, slugDir.Name())

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

				data, err := safeio.ReadFile(manifestPath)
				if err != nil {
					continue
				}

				var manifest client.BundleResolveResponse
				if err := json.Unmarshal(data, &manifest); err != nil {
					continue
				}

				var modTime time.Time
				if info, statErr := os.Stat(versionPath); statErr == nil {
					modTime = info.ModTime()
				}

				out = append(out, CachedBundle{
					Namespace:  nsEntry.Name(),
					Slug:       slugDir.Name(),
					Version:    versionDir.Name(),
					AssetCount: len(manifest.Manifest.Layers),
					ModTime:    modTime,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}

		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}

		return out[i].Version < out[j].Version
	})

	return out, nil
}

// ListCachedByRecency returns cached bundles sorted by directory modification
// time (most recent first).
func ListCachedByRecency() ([]CachedBundle, error) {
	all, err := ListCached()
	if err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ModTime.After(all[j].ModTime)
	})

	return all, nil
}
