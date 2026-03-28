package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/paths"
	"github.com/musher-dev/mush/internal/safeio"
)

// ErrNoAssets indicates that a bundle version was resolved but contains no downloadable assets.
var ErrNoAssets = errors.New("bundle has no downloadable assets")

// CacheDir returns the base cache directory for bundles.
func CacheDir() string {
	cacheDir, err := paths.BundleCacheDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(os.TempDir(), "musher", "bundles")
		}

		return filepath.Join(home, ".cache", "musher", "bundles")
	}

	return cacheDir
}

// CachePath returns the cache path for a specific bundle version.
func CachePath(namespace, slug, version string) string {
	return filepath.Join(CacheDir(), namespace, slug, version)
}

// IsCached checks if a bundle version is already cached.
func IsCached(namespace, slug, version string) bool {
	manifestPath := filepath.Join(CachePath(namespace, slug, version), "manifest.json")
	_, err := os.Stat(manifestPath)

	return err == nil
}

// cleanStalePartials removes leftover staging directories from interrupted downloads.
func cleanStalePartials(cachePath string) {
	parent := filepath.Dir(cachePath)
	base := filepath.Base(cachePath)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}

	prefix := base + ".partial."

	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			_ = os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}

// Pull resolves, downloads, and caches a bundle version.
// It first attempts the single-request pull endpoint (all assets inline),
// falling back to per-asset download if the pull endpoint is unavailable.
// Returns the resolve response, the cache path, and any error.
func Pull(ctx context.Context, c *client.Client, namespace, slug, version string, out *output.Writer) (*client.BundleResolveResponse, string, error) {
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

	resolved, err := c.ResolveBundle(ctx, namespace, slug, version)
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

	// Ensure CACHEDIR.TAG exists in the cache root.
	EnsureCacheDirTag()

	// 2. Check cache hit.
	cachePath := CachePath(namespace, slug, resolved.Version)
	if IsCached(namespace, slug, resolved.Version) {
		out.Success("Using cached bundle")
		logger.Info("bundle cache hit", slog.String("event.type", "bundle.cache.hit"), slog.Bool("bundle.cache_hit", true))

		return resolved, cachePath, nil
	}

	logger.Info("bundle cache miss", slog.String("event.type", "bundle.cache.miss"), slog.Bool("bundle.cache_hit", false))

	// 3. Try single-request pull endpoint first (all assets inline).
	pullResp, pullErr := c.PullBundle(ctx, namespace, slug, resolved.Version)
	if pullErr == nil && len(pullResp.Manifest) > 0 {
		cachePath, err = pullToCache(logger, resolved, pullResp, cachePath)
		if err != nil {
			return nil, "", err
		}

		out.Success("Downloaded %d assets", len(pullResp.Manifest))

		storeManifestAndRef(logger, c, namespace, slug, resolved)

		return resolved, cachePath, nil
	}

	if pullErr != nil {
		logger.Warn("pull endpoint unavailable, falling back to per-asset download",
			slog.String("error", pullErr.Error()),
		)
	}

	// 4. Fallback: per-asset download.
	if len(resolved.Manifest.Layers) == 0 {
		return nil, "", fmt.Errorf(
			"%s/%s v%s: %w", namespace, slug, resolved.Version, ErrNoAssets,
		)
	}

	cachePath, err = downloadAssetsToCache(ctx, c, logger, out, resolved, namespace, slug, cachePath)
	if err != nil {
		return nil, "", err
	}

	storeManifestAndRef(logger, c, namespace, slug, resolved)

	return resolved, cachePath, nil
}

// pullToCache writes assets from a pull response into the cache.
func pullToCache(logger *slog.Logger, resolved *client.BundleResolveResponse, pullResp *client.PullBundleResponse, cachePath string) (string, error) {
	logger.Info("using pull endpoint for bundle download",
		slog.String("event.type", "bundle.download.pull"),
		slog.Int("bundle.asset_count", len(pullResp.Manifest)),
	)

	cleanStalePartials(cachePath)

	if mkdirErr := os.MkdirAll(filepath.Dir(cachePath), 0o700); mkdirErr != nil {
		return "", fmt.Errorf("create cache parent: %w", mkdirErr)
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(cachePath), filepath.Base(cachePath)+".partial.")
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}

	stagingFailed := true

	defer func() {
		if stagingFailed {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	assetsDir := filepath.Join(stagingDir, "assets")
	if err := safeio.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging assets directory: %w", err)
	}

	// Backfill resolved manifest layers from pull response so the cached
	// manifest.json contains complete layer metadata for later use.
	if len(resolved.Manifest.Layers) == 0 {
		layers := make([]client.BundleLayer, len(pullResp.Manifest))
		for i, asset := range pullResp.Manifest {
			layers[i] = client.BundleLayer{
				LogicalPath: asset.LogicalPath,
				AssetType:   asset.AssetType,
			}
		}

		resolved.Manifest.Layers = layers
	}

	for _, asset := range pullResp.Manifest {
		if err := ValidateLogicalPath(asset.LogicalPath); err != nil {
			return "", fmt.Errorf("invalid logical path: %w", err)
		}

		data := []byte(asset.ContentText)

		destPath := filepath.Join(assetsDir, asset.LogicalPath)
		if err := safeio.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", fmt.Errorf("create asset directory: %w", err)
		}

		if err := safeio.WriteFile(destPath, data, 0o644); err != nil {
			return "", fmt.Errorf("write asset %s: %w", asset.LogicalPath, err)
		}

		// Best-effort blob store.
		if _, blobErr := StoreBlob(data); blobErr != nil {
			logger.Warn("blob store write failed", slog.String("error", blobErr.Error()))
		}
	}

	if err := writeManifest(stagingDir, resolved); err != nil {
		return "", err
	}

	if err := os.Rename(stagingDir, cachePath); err != nil {
		if IsCached(resolved.Namespace, resolved.Slug, resolved.Version) {
			_ = os.RemoveAll(stagingDir)
			stagingFailed = false

			return cachePath, nil
		}

		return "", fmt.Errorf("promote staging cache: %w", err)
	}

	stagingFailed = false

	return cachePath, nil
}

// downloadAssetsToCache downloads assets one-by-one and writes them to the cache.
func downloadAssetsToCache(ctx context.Context, c *client.Client, logger *slog.Logger, out *output.Writer, resolved *client.BundleResolveResponse, namespace, slug, cachePath string) (string, error) {
	cleanStalePartials(cachePath)

	if mkdirErr := os.MkdirAll(filepath.Dir(cachePath), 0o700); mkdirErr != nil {
		return "", fmt.Errorf("create cache parent: %w", mkdirErr)
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(cachePath), filepath.Base(cachePath)+".partial.")
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}

	stagingFailed := true

	defer func() {
		if stagingFailed {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	assetsDir := filepath.Join(stagingDir, "assets")
	if err := safeio.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging assets directory: %w", err)
	}

	// Per-asset API download (server proxies OCI content for registry bundles).
	spin := out.Spinner(fmt.Sprintf("Downloading %d assets", len(resolved.Manifest.Layers)))
	spin.Start()

	logger.Info("bundle asset download started", slog.String("event.type", "bundle.download.start"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

	for _, layer := range resolved.Manifest.Layers {
		if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
			spin.StopWithFailure("Path validation failed")
			logger.Error("bundle asset path validation failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", err.Error()))

			return "", fmt.Errorf("invalid logical path: %w", err)
		}

		if layer.AssetID == "" {
			spin.StopWithFailure("Asset metadata missing")
			logger.Error("bundle asset metadata missing", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath))

			return "", fmt.Errorf("asset %s is missing asset ID for API download", layer.LogicalPath)
		}

		data, fetchErr := c.FetchBundleAsset(ctx, layer.AssetID)
		if fetchErr != nil {
			// Fallback to hub asset-by-path endpoint (works for OCI-sourced bundles
			// where the runner endpoint may return 503).
			logger.Warn("runner asset endpoint failed, trying hub fallback",
				slog.String("bundle.asset.id", layer.AssetID),
				slog.String("bundle.asset.logical_path", layer.LogicalPath),
				slog.String("error", fetchErr.Error()),
			)

			data, fetchErr = c.FetchHubBundleAsset(ctx, namespace, slug, layer.LogicalPath, resolved.Version)
		}

		if fetchErr != nil {
			spin.StopWithFailure("Asset download failed")
			logger.Error("bundle asset download failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", fetchErr.Error()))

			return "", fmt.Errorf("fetch asset %s: %w", layer.LogicalPath, fetchErr)
		}

		// Verify SHA256 (may recover trailing newline stripped by server).
		verified, verifyErr := VerifySHA256(data, layer.ContentSHA256)
		if verifyErr != nil {
			spin.StopWithFailure("Integrity check failed")
			logger.Error("bundle asset integrity check failed",
				slog.String("event.type", "bundle.download.asset.error"),
				slog.String("bundle.asset.logical_path", layer.LogicalPath),
				slog.String("error", verifyErr.Error()),
				slog.Int("bundle.asset.size_got", len(data)),
			)

			return "", fmt.Errorf("asset %s: %w", layer.LogicalPath, verifyErr)
		}

		data = verified

		// Store in content-addressable blob store.
		if _, blobErr := StoreBlob(data); blobErr != nil {
			logger.Warn("blob store write failed", slog.String("error", blobErr.Error()))
		}

		// Write to staging cache (materialized view).
		destPath := filepath.Join(assetsDir, layer.LogicalPath)
		if err := safeio.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", fmt.Errorf("create asset directory: %w", err)
		}

		if err := safeio.WriteFile(destPath, data, 0o644); err != nil {
			return "", fmt.Errorf("write asset %s: %w", layer.LogicalPath, err)
		}
	}

	spin.StopWithSuccess(fmt.Sprintf("Downloaded %d assets", len(resolved.Manifest.Layers)))
	logger.Info("bundle asset download completed", slog.String("event.type", "bundle.download.complete"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

	if err := writeManifest(stagingDir, resolved); err != nil {
		return "", err
	}

	if err := os.Rename(stagingDir, cachePath); err != nil {
		if IsCached(namespace, slug, resolved.Version) {
			_ = os.RemoveAll(stagingDir)
			stagingFailed = false

			return cachePath, nil
		}

		return "", fmt.Errorf("promote staging cache: %w", err)
	}

	stagingFailed = false

	return cachePath, nil
}

// storeManifestAndRef persists manifest and ref pointers (best-effort).
func storeManifestAndRef(logger *slog.Logger, c *client.Client, namespace, slug string, resolved *client.BundleResolveResponse) {
	hostID := paths.HostIDFromURL(c.BaseURL())

	if storeErr := StoreManifest(hostID, namespace, slug, resolved.Version, resolved); storeErr != nil {
		logger.Warn("manifest store write failed", slog.String("error", storeErr.Error()))
	}

	if refErr := UpdateRef(hostID, namespace, slug, resolved.Version); refErr != nil {
		logger.Warn("ref update failed", slog.String("error", refErr.Error()))
	}
}

func writeManifest(cachePath string, resolved *client.BundleResolveResponse) error {
	manifestData, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := safeio.WriteFile(filepath.Join(cachePath, "manifest.json"), manifestData, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}
