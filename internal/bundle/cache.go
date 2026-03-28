package bundle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/paths"
)

// ErrNoAssets indicates that a bundle version was resolved but contains no downloadable assets.
var ErrNoAssets = errors.New("bundle has no downloadable assets")

// Pull resolves, downloads, and caches a bundle version.
// It first attempts the single-request pull endpoint (all assets inline),
// falling back to per-asset download if the pull endpoint is unavailable.
// Returns the resolve response and any error.
func Pull(ctx context.Context, c *client.Client, namespace, slug, version string, out *output.Writer) (*client.BundleResolveResponse, error) {
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

		return nil, fmt.Errorf("resolve bundle: %w", err)
	}

	logger.Info(
		"bundle resolved",
		slog.String("event.type", "bundle.resolve.ok"),
		slog.String("bundle.version", resolved.Version),
	)

	spin.StopWithSuccess(fmt.Sprintf("Resolved %s v%s", slug, resolved.Version))

	// Ensure CACHEDIR.TAG exists in the cache root.
	EnsureCacheDirTag()

	// 2. Check cache hit: manifest fresh + all blobs present.
	hostID := paths.HostIDFromURL(c.BaseURL())

	if HasManifest(hostID, namespace, slug, resolved.Version) &&
		HasAllBlobs(&resolved.Manifest) {
		out.Success("Using cached bundle")
		logger.Info("bundle cache hit", slog.String("event.type", "bundle.cache.hit"), slog.Bool("bundle.cache_hit", true))

		return resolved, nil
	}

	logger.Info("bundle cache miss", slog.String("event.type", "bundle.cache.miss"), slog.Bool("bundle.cache_hit", false))

	// 3. Try single-request pull endpoint first (all assets inline).
	pullResp, pullErr := c.PullBundle(ctx, namespace, slug, resolved.Version)
	if pullErr == nil && len(pullResp.Manifest) > 0 {
		if err := storePullResponse(logger, resolved, pullResp); err != nil {
			return nil, err
		}

		out.Success("Downloaded %d assets", len(pullResp.Manifest))

		storeManifestAndRef(logger, c, namespace, slug, resolved)

		return resolved, nil
	}

	if pullErr != nil {
		logger.Warn("pull endpoint unavailable, falling back to per-asset download",
			slog.String("error", pullErr.Error()),
		)
	}

	// 4. Fallback: per-asset download.
	if len(resolved.Manifest.Layers) == 0 {
		return nil, fmt.Errorf(
			"%s/%s v%s: %w", namespace, slug, resolved.Version, ErrNoAssets,
		)
	}

	if err := downloadAssetsToBlobs(ctx, c, logger, out, resolved, namespace, slug); err != nil {
		return nil, err
	}

	storeManifestAndRef(logger, c, namespace, slug, resolved)

	return resolved, nil
}

// storePullResponse writes assets from a pull response into the blob store.
func storePullResponse(logger *slog.Logger, resolved *client.BundleResolveResponse, pullResp *client.PullBundleResponse) error {
	logger.Info("using pull endpoint for bundle download",
		slog.String("event.type", "bundle.download.pull"),
		slog.Int("bundle.asset_count", len(pullResp.Manifest)),
	)

	// Backfill resolved manifest layers from pull response so the cached
	// manifest contains complete layer metadata for later use.
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
			return fmt.Errorf("invalid logical path: %w", err)
		}

		data := []byte(asset.ContentText)

		digest, blobErr := StoreBlob(data)
		if blobErr != nil {
			return fmt.Errorf("store blob for %s: %w", asset.LogicalPath, blobErr)
		}

		// Backfill digest into layers.
		for i := range resolved.Manifest.Layers {
			if resolved.Manifest.Layers[i].LogicalPath == asset.LogicalPath && resolved.Manifest.Layers[i].ContentSHA256 == "" {
				resolved.Manifest.Layers[i].ContentSHA256 = digest
				resolved.Manifest.Layers[i].SizeBytes = int64(len(data))
			}
		}
	}

	return nil
}

// downloadAssetsToBlobs downloads assets one-by-one and stores them as blobs.
func downloadAssetsToBlobs(ctx context.Context, c *client.Client, logger *slog.Logger, out *output.Writer, resolved *client.BundleResolveResponse, namespace, slug string) error {
	spin := out.Spinner(fmt.Sprintf("Downloading %d assets", len(resolved.Manifest.Layers)))
	spin.Start()

	logger.Info("bundle asset download started", slog.String("event.type", "bundle.download.start"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

	for _, layer := range resolved.Manifest.Layers {
		if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
			spin.StopWithFailure("Path validation failed")
			logger.Error("bundle asset path validation failed", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath), slog.String("error", err.Error()))

			return fmt.Errorf("invalid logical path: %w", err)
		}

		if layer.AssetID == "" {
			spin.StopWithFailure("Asset metadata missing")
			logger.Error("bundle asset metadata missing", slog.String("event.type", "bundle.download.asset.error"), slog.String("bundle.asset.logical_path", layer.LogicalPath))

			return fmt.Errorf("asset %s is missing asset ID for API download", layer.LogicalPath)
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

			return fmt.Errorf("fetch asset %s: %w", layer.LogicalPath, fetchErr)
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

			return fmt.Errorf("asset %s: %w", layer.LogicalPath, verifyErr)
		}

		data = verified

		// Store in content-addressable blob store.
		if _, blobErr := StoreBlob(data); blobErr != nil {
			logger.Warn("blob store write failed", slog.String("error", blobErr.Error()))
		}
	}

	spin.StopWithSuccess(fmt.Sprintf("Downloaded %d assets", len(resolved.Manifest.Layers)))
	logger.Info("bundle asset download completed", slog.String("event.type", "bundle.download.complete"), slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)))

	return nil
}

// storeManifestAndRef persists manifest, metadata sidecar, and ref pointers (best-effort).
func storeManifestAndRef(logger *slog.Logger, c *client.Client, namespace, slug string, resolved *client.BundleResolveResponse) {
	hostID := paths.HostIDFromURL(c.BaseURL())

	if storeErr := StoreManifest(hostID, namespace, slug, resolved.Version, resolved); storeErr != nil {
		logger.Warn("manifest store write failed", slog.String("error", storeErr.Error()))
	}

	meta := &ManifestMeta{
		FetchedAt: time.Now(),
		TTL:       DefaultManifestTTL,
	}

	if metaErr := StoreManifestMeta(hostID, namespace, slug, resolved.Version, meta); metaErr != nil {
		logger.Warn("manifest meta write failed", slog.String("error", metaErr.Error()))
	}

	if refErr := UpdateRef(hostID, namespace, slug, resolved.Version); refErr != nil {
		logger.Warn("ref update failed", slog.String("error", refErr.Error()))
	}
}
