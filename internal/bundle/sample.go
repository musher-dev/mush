package bundle

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/safeio"
)

//go:embed sample/skills/hello/SKILL.md
var sampleFS embed.FS

// sampleAssets defines the embedded assets and their types.
var sampleAssets = []struct {
	EmbedPath   string
	LogicalPath string
	AssetType   string
}{
	{
		EmbedPath:   "sample/skills/hello/SKILL.md",
		LogicalPath: "skills/hello/SKILL.md",
		AssetType:   "skill",
	},
}

// ExtractSampleBundle creates a temporary cache directory from the embedded sample bundle.
// Returns the synthetic resolve response, cache path, cleanup function, and any error.
func ExtractSampleBundle() (resolved *client.BundleResolveResponse, cachePath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "mush-sample-bundle-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	assetsDir := filepath.Join(tmpDir, "assets")

	var layers []client.BundleLayer

	for _, asset := range sampleAssets {
		data, readErr := sampleFS.ReadFile(asset.EmbedPath)
		if readErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("read embedded asset %s: %w", asset.EmbedPath, readErr)
		}

		destPath := filepath.Join(assetsDir, asset.LogicalPath)

		if mkErr := safeio.MkdirAll(filepath.Dir(destPath), 0o755); mkErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("create asset dir: %w", mkErr)
		}

		if wErr := safeio.WriteFile(destPath, data, 0o644); wErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("write asset %s: %w", asset.LogicalPath, wErr)
		}

		hash := sha256.Sum256(data)

		layers = append(layers, client.BundleLayer{
			LogicalPath:   asset.LogicalPath,
			AssetType:     asset.AssetType,
			ContentSHA256: fmt.Sprintf("%x", hash),
			SizeBytes:     int64(len(data)),
		})
	}

	resolved = &client.BundleResolveResponse{
		Namespace: "_local",
		Slug:      "sample",
		Version:   "0.0.0-sample",
		Ref:       "_local/sample",
		Manifest: client.BundleManifest{
			Layers: layers,
		},
	}

	if wErr := writeManifest(tmpDir, resolved); wErr != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("write manifest: %w", wErr)
	}

	return resolved, tmpDir, cleanup, nil
}
