package nav

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	harnesslib "github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/paths"
)

// --- Message types ---

// bundleResolvedMsg carries a successful resolve result.
type bundleResolvedMsg struct {
	namespace  string
	slug       string
	version    string
	assetCount int
	resolved   *client.BundleResolveResponse
}

// bundleResolveErrorMsg carries a resolve error.
type bundleResolveErrorMsg struct {
	err     error
	slug    string
	version string
}

// bundleCacheHitMsg indicates the bundle is already cached.
type bundleCacheHitMsg struct{}

// bundleDownloadProgressMsg reports download progress.
type bundleDownloadProgressMsg struct {
	current int
	total   int
	label   string
}

// bundleDownloadCompleteMsg indicates download is complete.
type bundleDownloadCompleteMsg struct{}

// bundleDownloadErrorMsg carries a download error.
type bundleDownloadErrorMsg struct {
	err error
}

// bundleInstallConflictsMsg carries the result of conflict detection.
type bundleInstallConflictsMsg struct {
	hasConflicts  bool
	conflictPaths []string
}

// bundleListLoadedMsg carries the async-loaded recent and installed bundle lists.
type bundleListLoadedMsg struct {
	recent    []recentBundleEntry
	installed []installedBundleEntry
}

// --- Commands ---

// cmdLoadBundleLists loads recent cached bundles and installed bundles in the working directory.
func cmdLoadBundleLists(workDir string) tea.Cmd {
	return func() tea.Msg {
		var recent []recentBundleEntry

		cached, err := bundle.ListCachedByRecency()
		if err == nil {
			for _, c := range cached {
				recent = append(recent, recentBundleEntry{
					namespace: c.Namespace,
					slug:      c.Slug,
					version:   c.Version,
					timeAgo:   formatTimeAgo(c.FetchedAt),
				})
			}
		}

		var installed []installedBundleEntry

		if workDir != "" {
			bundles, loadErr := bundle.LoadInstalled(workDir)
			if loadErr == nil {
				for i := range bundles {
					installed = append(installed, installedBundleEntry{
						namespace: bundles[i].Namespace,
						slug:      bundles[i].Slug,
						ref:       bundles[i].Ref,
						version:   bundles[i].Version,
						harness:   bundles[i].Harness,
					})
				}
			}
		}

		return bundleListLoadedMsg{
			recent:    recent,
			installed: installed,
		}
	}
}

// cmdResolveBundle resolves a bundle slug/version via the API.
func cmdResolveBundle(ctx context.Context, c *client.Client, namespace, slug, version string) tea.Cmd {
	return func() tea.Msg {
		resolved, err := c.ResolveBundle(navBaseCtx(ctx), namespace, slug, version)
		if err != nil {
			return bundleResolveErrorMsg{
				err:     err,
				slug:    slug,
				version: version,
			}
		}

		return bundleResolvedMsg{
			namespace:  namespace,
			slug:       slug,
			version:    resolved.Version,
			assetCount: len(resolved.Manifest.Layers),
			resolved:   resolved,
		}
	}
}

// cmdCheckBundleCache checks if the bundle is cached (manifest + all blobs); if so, returns a cache hit.
// Otherwise, starts downloading assets.
func cmdCheckBundleCache(ctx context.Context, deps *Dependencies, namespace, slug, version string) tea.Cmd {
	return func() tea.Msg {
		if deps == nil || deps.Client == nil {
			return bundleDownloadErrorMsg{
				err: fmt.Errorf("client not available"),
			}
		}

		hostID := paths.HostIDFromURL(deps.Client.BaseURL())

		// Check cache: manifest exists + all blobs present.
		if bundle.HasManifest(hostID, namespace, slug, version) {
			manifest, loadErr := bundle.LoadManifest(hostID, namespace, slug, version)
			if loadErr == nil && bundle.HasAllBlobs(&manifest.Manifest) {
				return bundleCacheHitMsg{}
			}
		}

		// Need to download — resolve first to get manifest.
		resolveCtx := navBaseCtx(ctx)

		resolved, err := deps.Client.ResolveBundle(resolveCtx, namespace, slug, version)
		if err != nil {
			return bundleDownloadErrorMsg{
				err: fmt.Errorf("resolve bundle: %w", err),
			}
		}

		if err := downloadBundle(resolveCtx, deps.Client, resolved); err != nil {
			return bundleDownloadErrorMsg{
				err: err,
			}
		}

		return bundleDownloadCompleteMsg{}
	}
}

// cmdCheckInstallConflicts checks which target files already exist in workDir.
func cmdCheckInstallConflicts(namespace, slug, version, harnessName, workDir string, c *client.Client) tea.Cmd {
	return func() tea.Msg {
		manifest, err := loadManifestFromStore(c, namespace, slug, version)
		if err != nil {
			return bundleInstallConflictsMsg{}
		}

		spec, ok := harnesslib.GetProvider(harnessName)
		if !ok {
			return bundleInstallConflictsMsg{}
		}

		mapper := bundle.NewProviderMapper(spec)

		var conflicts []string

		for _, layer := range manifest.Manifest.Layers {
			targetPath, mapErr := mapper.MapAsset(workDir, &layer)
			if mapErr != nil || targetPath == "" {
				continue
			}

			if _, statErr := os.Stat(targetPath); statErr == nil {
				conflicts = append(conflicts, targetPath)
			}
		}

		return bundleInstallConflictsMsg{
			hasConflicts:  len(conflicts) > 0,
			conflictPaths: conflicts,
		}
	}
}

// loadManifestFromStore reads a manifest from the content-addressable store.
func loadManifestFromStore(c *client.Client, namespace, slug, version string) (*client.BundleResolveResponse, error) {
	var hostID string
	if c != nil {
		hostID = paths.HostIDFromURL(c.BaseURL())
	}

	resolved, err := bundle.LoadManifest(hostID, namespace, slug, version)
	if err != nil {
		return nil, fmt.Errorf("load manifest from store: %w", err)
	}

	return resolved, nil
}

// downloadBundle downloads all assets and stores them as blobs.
// It tries the single-request pull endpoint first, falling back to per-asset download.
func downloadBundle(ctx context.Context, c *client.Client, resolved *client.BundleResolveResponse) error {
	// Try single-request pull endpoint first.
	pullResp, pullErr := c.PullBundle(ctx, resolved.Namespace, resolved.Slug, resolved.Version)
	if pullErr == nil && len(pullResp.Manifest) > 0 {
		return downloadBundleFromPull(resolved, pullResp, c)
	}

	// Fallback: per-asset download.
	if len(resolved.Manifest.Layers) == 0 {
		return fmt.Errorf("bundle has no downloadable assets")
	}

	return downloadBundlePerAsset(ctx, c, resolved)
}

func downloadBundleFromPull(resolved *client.BundleResolveResponse, pullResp *client.PullBundleResponse, c *client.Client) error {
	// Backfill resolved manifest layers from pull response.
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
		if validateErr := bundle.ValidateLogicalPath(asset.LogicalPath); validateErr != nil {
			return fmt.Errorf("invalid logical path %q: %w", asset.LogicalPath, validateErr)
		}

		data := []byte(asset.ContentText)

		digest, blobErr := bundle.StoreBlob(data)
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

	storeManifestAndRef(c, resolved)

	return nil
}

func downloadBundlePerAsset(ctx context.Context, c *client.Client, resolved *client.BundleResolveResponse) error {
	for _, layer := range resolved.Manifest.Layers {
		if validateErr := bundle.ValidateLogicalPath(layer.LogicalPath); validateErr != nil {
			return fmt.Errorf("invalid logical path %q: %w", layer.LogicalPath, validateErr)
		}

		if layer.AssetID == "" {
			return fmt.Errorf("asset %s is missing asset ID", layer.LogicalPath)
		}

		data, fetchErr := c.FetchBundleAsset(ctx, layer.AssetID)
		if fetchErr != nil {
			data, fetchErr = c.FetchHubBundleAsset(ctx, resolved.Namespace, resolved.Slug, layer.LogicalPath, resolved.Version)
		}

		if fetchErr != nil {
			return fmt.Errorf("fetch asset %s: %w", layer.LogicalPath, fetchErr)
		}

		if layer.ContentSHA256 != "" {
			verified, verifyErr := bundle.VerifySHA256(data, layer.ContentSHA256)
			if verifyErr != nil {
				return fmt.Errorf("asset %s: %w", layer.LogicalPath, verifyErr)
			}

			data = verified
		}

		if _, blobErr := bundle.StoreBlob(data); blobErr != nil {
			return fmt.Errorf("store blob for %s: %w", layer.LogicalPath, blobErr)
		}
	}

	storeManifestAndRef(c, resolved)

	return nil
}

// storeManifestAndRef persists manifest, metadata sidecar, and ref to the store (best-effort).
func storeManifestAndRef(c *client.Client, resolved *client.BundleResolveResponse) {
	hostID := paths.HostIDFromURL(c.BaseURL())

	_ = bundle.StoreManifest(hostID, resolved.Namespace, resolved.Slug, resolved.Version, resolved)

	_ = bundle.StoreManifestMeta(hostID, resolved.Namespace, resolved.Slug, resolved.Version, &bundle.ManifestMeta{
		FetchedAt: time.Now(),
		TTL:       bundle.DefaultManifestTTL,
	})

	_ = bundle.UpdateRef(hostID, resolved.Namespace, resolved.Slug, resolved.Version)
}
