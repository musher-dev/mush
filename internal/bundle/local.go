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
//   - Bare: directory contains files directly (symlinked into a temp cache structure)
//
// Returns the synthetic resolve response, cache path, cleanup function, and any error.
//
//nolint:gocritic // unnamedResult: four returns match the pattern used across the package
func LoadFromDir(dirPath string) (*client.BundleResolveResponse, string, func(), error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("directory not found: %w", err)
	}

	if !info.IsDir() {
		return nil, "", nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	assetsDir := filepath.Join(absDir, "assets")

	if stat, sErr := os.Stat(assetsDir); sErr == nil && stat.IsDir() {
		return loadCacheCompatible(absDir, assetsDir)
	}

	return loadBareDir(absDir)
}

// loadCacheCompatible handles directories that already have assets/ layout.
//
//nolint:gocritic // unnamedResult: internal helper, signature matches LoadFromDir
func loadCacheCompatible(absDir, assetsDir string) (*client.BundleResolveResponse, string, func(), error) {
	// Check for existing manifest.json.
	manifestPath := filepath.Join(absDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err == nil {
		// Read existing manifest.
		data, readErr := safeio.ReadFile(manifestPath)
		if readErr != nil {
			return nil, "", nil, fmt.Errorf("read manifest.json: %w", readErr)
		}

		var resolved client.BundleResolveResponse
		if jsonErr := decodeJSONBytes(data, &resolved); jsonErr != nil {
			return nil, "", nil, fmt.Errorf("parse manifest.json: %w", jsonErr)
		}

		return &resolved, absDir, func() {}, nil
	}

	// No manifest — scan assets/ to build one.
	layers, err := scanAssetsDir(assetsDir)
	if err != nil {
		return nil, "", nil, err
	}

	if len(layers) == 0 {
		return nil, "", nil, fmt.Errorf("no recognized bundle assets found in %s", assetsDir)
	}

	resolved := syntheticResolveResponse(filepath.Base(absDir), layers)

	// Write manifest for future cache hits.
	if wErr := writeManifest(absDir, resolved); wErr != nil {
		return nil, "", nil, fmt.Errorf("write manifest: %w", wErr)
	}

	return resolved, absDir, func() {}, nil
}

// loadBareDir handles directories without assets/ layout by creating a temp cache structure.
//
//nolint:gocritic // unnamedResult: internal helper, signature matches LoadFromDir
func loadBareDir(absDir string) (*client.BundleResolveResponse, string, func(), error) {
	// Scan the directory for recognizable assets.
	layers, filePaths, err := scanBareDir(absDir)
	if err != nil {
		return nil, "", nil, err
	}

	if len(layers) == 0 {
		return nil, "", nil, fmt.Errorf("no recognized bundle assets found in %s", absDir)
	}

	// Create temp dir with cache structure.
	tmpDir, err := os.MkdirTemp("", "mush-local-bundle-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	assetsDir := filepath.Join(tmpDir, "assets")

	for i, layer := range layers {
		destPath := filepath.Join(assetsDir, layer.LogicalPath)

		if mkErr := safeio.MkdirAll(filepath.Dir(destPath), 0o755); mkErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("create asset dir: %w", mkErr)
		}

		if linkErr := os.Symlink(filePaths[i], destPath); linkErr != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("symlink asset %s: %w", layer.LogicalPath, linkErr)
		}
	}

	resolved := syntheticResolveResponse(filepath.Base(absDir), layers)

	if wErr := writeManifest(tmpDir, resolved); wErr != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("write manifest: %w", wErr)
	}

	return resolved, tmpDir, cleanup, nil
}

// scanAssetsDir walks an assets/ directory and builds BundleLayers.
func scanAssetsDir(assetsDir string) ([]client.BundleLayer, error) {
	var layers []client.BundleLayer

	err := filepath.WalkDir(assetsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(assetsDir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		assetType := inferAssetType(relPath)
		if assetType == "" {
			return nil // skip unrecognized files
		}

		layer, layerErr := buildLayer(path, relPath, assetType)
		if layerErr != nil {
			return layerErr
		}

		layers = append(layers, layer)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan assets directory: %w", err)
	}

	return layers, nil
}

// scanBareDir walks a bare directory (no assets/ subdirectory) and builds BundleLayers.
// Returns layers and the corresponding absolute file paths.
func scanBareDir(dir string) ([]client.BundleLayer, []string, error) {
	var (
		layers    []client.BundleLayer
		filePaths []string
	)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}

		assetType := inferAssetType(relPath)
		if assetType == "" {
			return nil // skip unrecognized files
		}

		layer, layerErr := buildLayer(path, relPath, assetType)
		if layerErr != nil {
			return layerErr
		}

		layers = append(layers, layer)
		filePaths = append(filePaths, path)

		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan directory: %w", err)
	}

	return layers, filePaths, nil
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

// buildLayer creates a BundleLayer from a file on disk.
func buildLayer(absPath, logicalPath, assetType string) (client.BundleLayer, error) {
	data, err := safeio.ReadFile(absPath)
	if err != nil {
		return client.BundleLayer{}, fmt.Errorf("read %s: %w", logicalPath, err)
	}

	hash := sha256.Sum256(data)

	return client.BundleLayer{
		LogicalPath:   logicalPath,
		AssetType:     assetType,
		ContentSHA256: fmt.Sprintf("%x", hash),
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
func decodeJSONBytes(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil { //nolint:musttag // using any
		return fmt.Errorf("json unmarshal: %w", err)
	}

	return nil
}
