package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/output"
)

// CacheDir returns the base cache directory for bundles.
func CacheDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

	return filepath.Join(configDir, "mush", "cache")
}

// CachePath returns the cache path for a specific bundle version.
func CachePath(workspace, slug, version string) string {
	return filepath.Join(CacheDir(), workspace, slug, version)
}

// IsCached checks if a bundle version is already cached.
func IsCached(workspace, slug, version string) bool {
	manifestPath := filepath.Join(CachePath(workspace, slug, version), "manifest.json")
	_, err := os.Stat(manifestPath)

	return err == nil
}

// Pull resolves, downloads, and caches a bundle version.
// It tries OCI pull first (if oci_ref present), falling back to per-asset API download.
// Returns the resolve response, the cache path, and any error.
func Pull(ctx context.Context, c *client.Client, workspace, slug, version string, out *output.Writer) (*client.BundleResolveResponse, string, error) {
	// 1. Resolve bundle via API.
	spin := out.Spinner("Resolving bundle")
	spin.Start()

	resolved, err := c.ResolveBundle(ctx, slug, version)
	if err != nil {
		spin.StopWithFailure("Failed to resolve bundle")
		return nil, "", fmt.Errorf("resolve bundle: %w", err)
	}

	spin.StopWithSuccess(fmt.Sprintf("Resolved %s v%s", slug, resolved.Version))

	// 2. Check cache hit.
	cachePath := CachePath(workspace, slug, resolved.Version)
	if IsCached(workspace, slug, resolved.Version) {
		out.Success("Using cached bundle")
		return resolved, cachePath, nil
	}

	// 3. Download assets.
	assetsDir := filepath.Join(cachePath, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil { //nolint:gosec // G301: cache dir needs 0o755 for accessibility
		return nil, "", fmt.Errorf("create cache directory: %w", err)
	}

	// Try OCI pull first if oci_ref is present.
	if resolved.OCIRef != "" {
		spin = out.Spinner("Pulling from OCI registry")
		spin.Start()

		ociManifest, ociErr := PullOCI(
			ctx,
			resolved.OCIRef,
			resolved.OCIDigest,
			assetsDir,
		)
		if ociErr == nil {
			if len(resolved.Manifest.Layers) == 0 && ociManifest != nil {
				resolved.Manifest = *ociManifest
			}

			spin.StopWithSuccess("Pulled from OCI registry")
			// Write manifest.
			if err := writeManifest(cachePath, resolved); err != nil {
				return nil, "", err
			}

			return resolved, cachePath, nil
		}

		// OCI pull failed, fall back to API.
		spin.StopWithFailure("OCI pull failed, falling back to API")
	}

	if len(resolved.Manifest.Layers) == 0 {
		return nil, "", fmt.Errorf(
			"bundle resolution did not include OCI reference or asset manifest metadata; unable to download bundle contents",
		)
	}

	// Per-asset API download.
	spin = out.Spinner(fmt.Sprintf("Downloading %d assets", len(resolved.Manifest.Layers)))
	spin.Start()

	for _, layer := range resolved.Manifest.Layers {
		if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
			spin.StopWithFailure("Path validation failed")
			return nil, "", fmt.Errorf("invalid logical path: %w", err)
		}

		if layer.AssetID == "" {
			spin.StopWithFailure("Asset metadata missing")
			return nil, "", fmt.Errorf("asset %s is missing asset ID for API download", layer.LogicalPath)
		}

		data, fetchErr := c.FetchBundleAsset(ctx, layer.AssetID)
		if fetchErr != nil {
			spin.StopWithFailure("Asset download failed")
			return nil, "", fmt.Errorf("fetch asset %s: %w", layer.AssetID, fetchErr)
		}

		// Verify SHA256.
		if err := verifySHA256(data, layer.ContentSHA256); err != nil {
			spin.StopWithFailure("Integrity check failed")
			return nil, "", fmt.Errorf("asset %s: %w", layer.LogicalPath, err)
		}

		// Write to cache.
		destPath := filepath.Join(assetsDir, layer.LogicalPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil { //nolint:gosec // G301: cache subdirs
			return nil, "", fmt.Errorf("create asset directory: %w", err)
		}

		if err := os.WriteFile(destPath, data, 0o644); err != nil { //nolint:gosec // G306: cache files are readable
			return nil, "", fmt.Errorf("write asset %s: %w", layer.LogicalPath, err)
		}
	}

	spin.StopWithSuccess(fmt.Sprintf("Downloaded %d assets", len(resolved.Manifest.Layers)))

	// Write manifest.
	if err := writeManifest(cachePath, resolved); err != nil {
		return nil, "", err
	}

	return resolved, cachePath, nil
}

func writeManifest(cachePath string, resolved *client.BundleResolveResponse) error {
	manifestData, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(cachePath, "manifest.json"), manifestData, 0o644); err != nil { //nolint:gosec // G306: manifest is readable
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}
