package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/paths"
)

// CacheDir returns the base cache directory for bundles.
func CacheDir() string {
	cacheDir, err := paths.BundleCacheDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".cache", "mush", "cache")
	}

	return cacheDir
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
	logger := observability.FromContext(ctx).With(
		slog.String("component", "bundle"),
		slog.String("bundle.slug", slug),
	)
	if version != "" {
		logger = logger.With(slog.String("bundle.version", version))
	}

	logger.Info("resolving bundle", slog.String("event.type", "bundle.resolve.start"))

	// 1. Resolve bundle via API.
	spin := out.Spinner("Resolving bundle")
	spin.Start()

	resolved, err := c.ResolveBundle(ctx, slug, version)
	if err != nil {
		spin.StopWithFailure("Failed to resolve bundle")
		logger.Error("bundle resolution failed", slog.String("event.type", "bundle.resolve.error"), slog.String("error", err.Error()))

		return nil, "", fmt.Errorf("resolve bundle: %w", err)
	}

	logger.Info(
		"bundle resolved",
		slog.String("event.type", "bundle.resolve.ok"),
		slog.String("bundle.version", resolved.Version),
	)

	spin.StopWithSuccess(fmt.Sprintf("Resolved %s v%s", slug, resolved.Version))

	// 2. Check cache hit.
	cachePath := CachePath(workspace, slug, resolved.Version)
	if IsCached(workspace, slug, resolved.Version) {
		out.Success("Using cached bundle")
		logger.Info("bundle cache hit", slog.String("event.type", "bundle.cache.hit"), slog.Bool("bundle.cache_hit", true))

		return resolved, cachePath, nil
	}

	logger.Info("bundle cache miss", slog.String("event.type", "bundle.cache.miss"), slog.Bool("bundle.cache_hit", false))

	// 3. Download assets.
	assetsDir := filepath.Join(cachePath, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil { //nolint:gosec // G301: cache dir needs 0o755 for accessibility
		return nil, "", fmt.Errorf("create cache directory: %w", err)
	}

	// Try OCI pull first if oci_ref is present.
	if resolved.OCIRef != "" {
		logger.Info("starting OCI pull", slog.String("event.type", "bundle.pull.oci.start"))

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
			logger.Info("OCI pull completed", slog.String("event.type", "bundle.pull.oci.ok"))
			// Write manifest.
			if err := writeManifest(cachePath, resolved); err != nil {
				return nil, "", err
			}

			return resolved, cachePath, nil
		}

		// OCI pull failed, fall back to API.
		spin.StopWithFailure("OCI pull failed, falling back to API")
		logger.Warn("OCI pull failed, using API fallback", slog.String("event.type", "bundle.pull.oci.fallback"), slog.String("error", ociErr.Error()))
	}

	if len(resolved.Manifest.Layers) == 0 {
		return nil, "", fmt.Errorf(
			"bundle resolution did not include OCI reference or asset manifest metadata; unable to download bundle contents",
		)
	}

	// Per-asset API download.
	spin = out.Spinner(fmt.Sprintf("Downloading %d assets", len(resolved.Manifest.Layers)))
	spin.Start()
	logger.Info("bundle asset download started", slog.String("event.type", "bundle.download.start"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

	for _, layer := range resolved.Manifest.Layers {
		if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
			spin.StopWithFailure("Path validation failed")
			logger.Error("bundle asset path validation failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", err.Error()))

			return nil, "", fmt.Errorf("invalid logical path: %w", err)
		}

		if layer.AssetID == "" {
			spin.StopWithFailure("Asset metadata missing")
			logger.Error("bundle asset metadata missing", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath))

			return nil, "", fmt.Errorf("asset %s is missing asset ID for API download", layer.LogicalPath)
		}

		data, fetchErr := c.FetchBundleAsset(ctx, layer.AssetID)
		if fetchErr != nil {
			spin.StopWithFailure("Asset download failed")
			logger.Error("bundle asset download failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", fetchErr.Error()))

			return nil, "", fmt.Errorf("fetch asset %s: %w", layer.AssetID, fetchErr)
		}

		// Verify SHA256.
		if err := verifySHA256(data, layer.ContentSHA256); err != nil {
			spin.StopWithFailure("Integrity check failed")
			logger.Error("bundle asset integrity check failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", err.Error()))

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
	logger.Info("bundle asset download completed", slog.String("event.type", "bundle.download.complete"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

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
