package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/safeio"
)

// CachedBundle is a locally cached bundle version.
type CachedBundle struct {
	Namespace  string
	Slug       string
	Version    string
	HostID     string
	AssetCount int
	FetchedAt  time.Time
}

// ListCached returns all cached bundle versions by walking the manifests/ directory.
func ListCached() ([]CachedBundle, error) {
	root := filepath.Join(cacheRootDir(), "manifests")

	hostEntries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read manifests root: %w", err)
	}

	var out []CachedBundle

	for _, hostEntry := range hostEntries {
		if !hostEntry.IsDir() {
			continue
		}

		hostPath := filepath.Join(root, hostEntry.Name())

		nsEntries, err := os.ReadDir(hostPath)
		if err != nil {
			continue
		}

		for _, nsEntry := range nsEntries {
			if !nsEntry.IsDir() {
				continue
			}

			nsPath := filepath.Join(hostPath, nsEntry.Name())

			slugEntries, err := os.ReadDir(nsPath)
			if err != nil {
				continue
			}

			for _, slugEntry := range slugEntries {
				if !slugEntry.IsDir() {
					continue
				}

				slugPath := filepath.Join(nsPath, slugEntry.Name())

				versionFiles, err := os.ReadDir(slugPath)
				if err != nil {
					continue
				}

				for _, dirEntry := range versionFiles {
					if dirEntry.IsDir() || !strings.HasSuffix(dirEntry.Name(), ".json") || strings.HasSuffix(dirEntry.Name(), ".meta.json") {
						continue
					}

					version := strings.TrimSuffix(dirEntry.Name(), ".json")

					data, readErr := safeio.ReadFile(filepath.Join(slugPath, dirEntry.Name()))
					if readErr != nil {
						continue
					}

					var manifest client.BundleResolveResponse
					if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
						continue
					}

					// Try to read FetchedAt from the metadata sidecar.
					var fetchedAt time.Time

					metaPath := filepath.Join(slugPath, version+".meta.json")

					metaData, metaErr := safeio.ReadFile(metaPath)
					if metaErr == nil {
						var meta ManifestMeta
						if jsonErr := json.Unmarshal(metaData, &meta); jsonErr == nil {
							fetchedAt = meta.FetchedAt
						}
					}

					out = append(out, CachedBundle{
						Namespace:  nsEntry.Name(),
						Slug:       slugEntry.Name(),
						Version:    version,
						HostID:     hostEntry.Name(),
						AssetCount: len(manifest.Manifest.Layers),
						FetchedAt:  fetchedAt,
					})
				}
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

// ListCachedByRecency returns cached bundles sorted by FetchedAt
// time (most recent first).
func ListCachedByRecency() ([]CachedBundle, error) {
	all, err := ListCached()
	if err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].FetchedAt.After(all[j].FetchedAt)
	})

	return all, nil
}
