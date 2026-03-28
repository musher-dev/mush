package bundle

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/safeio"
)

// LoadFromDir loads a bundle from a local directory, bypassing the API.
// It supports two layouts:
//   - Cache-compatible: directory contains an assets/ subdirectory
//   - Bare: directory contains files directly
//
// Each scanned asset is stored as a blob so that ReadAsset works.
// Returns the synthetic resolve response and any error.
func LoadFromDir(dirPath string) (resolved *client.BundleResolveResponse, err error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	assetsDir := filepath.Join(absDir, "assets")

	if stat, sErr := os.Stat(assetsDir); sErr == nil && stat.IsDir() {
		return loadCacheCompatible(absDir, assetsDir)
	}

	return loadBareDir(absDir)
}

// loadCacheCompatible handles directories that already have assets/ layout.
func loadCacheCompatible(absDir, assetsDir string) (resolved *client.BundleResolveResponse, err error) {
	// Check for existing manifest.json.
	manifestPath := filepath.Join(absDir, "manifest.json")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		// Read existing manifest.
		data, readErr := safeio.ReadFile(manifestPath)
		if readErr != nil {
			return nil, fmt.Errorf("read manifest.json: %w", readErr)
		}

		var resolved client.BundleResolveResponse
		if jsonErr := decodeJSONBytes(data, &resolved); jsonErr != nil {
			return nil, fmt.Errorf("parse manifest.json: %w", jsonErr)
		}

		// Store blobs for all layers so ReadAsset works.
		if storeErr := storeBlobsFromAssetsDir(assetsDir, resolved.Manifest.Layers); storeErr != nil {
			return nil, storeErr
		}

		return &resolved, nil
	}

	// No manifest — scan assets/ to build one.
	layers, err := scanAssetsDir(assetsDir)
	if err != nil {
		return nil, err
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no recognized bundle assets found in %s", assetsDir)
	}

	resolved = syntheticResolveResponse(filepath.Base(absDir), layers)

	return resolved, nil
}

// loadBareDir handles directories without assets/ layout.
func loadBareDir(absDir string) (resolved *client.BundleResolveResponse, err error) {
	layers, err := scanBareDirAndStoreBlobs(absDir)
	if err != nil {
		return nil, err
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no recognized bundle assets found in %s", absDir)
	}

	resolved = syntheticResolveResponse(filepath.Base(absDir), layers)

	return resolved, nil
}

// storeBlobsFromAssetsDir stores blobs for each layer from an assets/ directory.
func storeBlobsFromAssetsDir(assetsDir string, layers []client.BundleLayer) error {
	for _, layer := range layers {
		srcPath := filepath.Join(assetsDir, layer.LogicalPath)

		data, readErr := safeio.ReadFile(srcPath)
		if readErr != nil {
			continue // skip missing files
		}

		if _, blobErr := StoreBlob(data); blobErr != nil {
			return fmt.Errorf("store blob for %s: %w", layer.LogicalPath, blobErr)
		}
	}

	return nil
}

// walkDirAndStoreBlobs is a common helper for scanning directories and storing blobs.
// It walks the directory, infers asset types, builds layers, and stores each asset as a blob.
func walkDirAndStoreBlobs(baseDir string) ([]client.BundleLayer, error) {
	var layers []client.BundleLayer

	err := filepath.WalkDir(baseDir, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if dirEntry.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		assetType := inferAssetType(relPath)
		if assetType == "" {
			return nil // skip unrecognized files
		}

		layer, layerErr := buildLayerAndStoreBlob(path, relPath, assetType)
		if layerErr != nil {
			return layerErr
		}

		layers = append(layers, layer)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}

	return layers, nil
}

// scanAssetsDir walks an assets/ directory and builds BundleLayers, storing blobs.
func scanAssetsDir(assetsDir string) ([]client.BundleLayer, error) {
	return walkDirAndStoreBlobs(assetsDir)
}

// scanBareDirAndStoreBlobs walks a bare directory and builds BundleLayers, storing blobs.
func scanBareDirAndStoreBlobs(dir string) ([]client.BundleLayer, error) {
	return walkDirAndStoreBlobs(dir)
}

// inferAssetType determines the asset type from a relative path.
func inferAssetType(relPath string) string {
	normalized := filepath.ToSlash(relPath)
	base := filepath.Base(normalized)

	switch {
	case strings.HasPrefix(normalized, "skills/") || base == "SKILL.md":
		return "skill"
	case strings.HasPrefix(normalized, "agents/") || base == "AGENT.md":
		return "agent_definition"
	case base == ".mcp.json" || base == "mcp.json" ||
		strings.HasPrefix(normalized, "tools/") ||
		(strings.HasSuffix(base, ".toml") && strings.Contains(normalized, "tools")):
		return "tool_config"
	default:
		return ""
	}
}

// buildLayerAndStoreBlob creates a BundleLayer from a file on disk and stores it as a blob.
func buildLayerAndStoreBlob(absPath, logicalPath, assetType string) (client.BundleLayer, error) {
	data, err := safeio.ReadFile(absPath)
	if err != nil {
		return client.BundleLayer{}, fmt.Errorf("read %s: %w", logicalPath, err)
	}

	hash := sha256.Sum256(data)
	digest := fmt.Sprintf("%x", hash)

	if _, blobErr := StoreBlob(data); blobErr != nil {
		return client.BundleLayer{}, fmt.Errorf("store blob for %s: %w", logicalPath, blobErr)
	}

	return client.BundleLayer{
		LogicalPath:   logicalPath,
		AssetType:     assetType,
		ContentSHA256: digest,
		SizeBytes:     int64(len(data)),
	}, nil
}

// syntheticResolveResponse creates a BundleResolveResponse for local bundles.
func syntheticResolveResponse(slug string, layers []client.BundleLayer) *client.BundleResolveResponse {
	return &client.BundleResolveResponse{
		Namespace: "_local",
		Slug:      slug,
		Version:   "0.0.0-local",
		Ref:       "_local/" + slug,
		Manifest: client.BundleManifest{
			Layers: layers,
		},
	}
}

// decodeJSONBytes unmarshals JSON data into a value.
func decodeJSONBytes[T any](data []byte, v *T) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}

	return nil
}
