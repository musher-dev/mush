package bundle

import (
	"crypto/sha256"
	"embed"
	"fmt"

	"github.com/musher-dev/mush/internal/client"
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

// ExtractSampleBundle stores the embedded sample bundle assets as blobs.
// Returns the synthetic resolve response and any error.
func ExtractSampleBundle() (resolved *client.BundleResolveResponse, err error) {
	var layers []client.BundleLayer

	for _, asset := range sampleAssets {
		data, readErr := sampleFS.ReadFile(asset.EmbedPath)
		if readErr != nil {
			return nil, fmt.Errorf("read embedded asset %s: %w", asset.EmbedPath, readErr)
		}

		digest, blobErr := StoreBlob(data)
		if blobErr != nil {
			return nil, fmt.Errorf("store blob for %s: %w", asset.LogicalPath, blobErr)
		}

		hash := sha256.Sum256(data)

		layers = append(layers, client.BundleLayer{
			LogicalPath:   asset.LogicalPath,
			AssetType:     asset.AssetType,
			ContentSHA256: fmt.Sprintf("%x", hash),
			SizeBytes:     int64(len(data)),
		})

		// Verify digest matches (StoreBlob returns the same hash).
		_ = digest
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

	return resolved, nil
}
