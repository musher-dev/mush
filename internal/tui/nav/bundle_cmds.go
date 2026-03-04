package nav

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
)

// --- Message types ---

// bundleResolvedMsg carries a successful resolve result.
type bundleResolvedMsg struct {
	namespace  string
	slug       string
	version    string
	assetCount int
	harness    string
	resolved   *client.BundleResolveResponse
}

// bundleResolveErrorMsg carries a resolve error.
type bundleResolveErrorMsg struct {
	err     error
	slug    string
	version string
	harness string
}

// bundleCacheHitMsg indicates the bundle is already cached.
type bundleCacheHitMsg struct {
	cachePath string
	harness   string
}

// bundleDownloadProgressMsg reports download progress.
type bundleDownloadProgressMsg struct {
	current int
	total   int
	label   string
}

// bundleDownloadCompleteMsg indicates download is complete.
type bundleDownloadCompleteMsg struct {
	cachePath string
	harness   string
}

// bundleDownloadErrorMsg carries a download error.
type bundleDownloadErrorMsg struct {
	err     error
	harness string
}

// --- Commands ---

// cmdResolveBundle resolves a bundle slug/version via the API.
func cmdResolveBundle(c *client.Client, namespace, slug, version, harness string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		resolved, err := c.ResolveBundle(ctx, namespace, slug, version)
		if err != nil {
			return bundleResolveErrorMsg{
				err:     err,
				slug:    slug,
				version: version,
				harness: harness,
			}
		}

		return bundleResolvedMsg{
			namespace:  namespace,
			slug:       slug,
			version:    resolved.Version,
			assetCount: len(resolved.Manifest.Layers),
			harness:    harness,
			resolved:   resolved,
		}
	}
}

// cmdCheckBundleCache checks if the bundle is cached; if so, returns a cache hit.
// Otherwise, starts downloading assets.
func cmdCheckBundleCache(deps *Dependencies, namespace, slug, version, harness string) tea.Cmd {
	return func() tea.Msg {
		if deps == nil || deps.Client == nil {
			return bundleDownloadErrorMsg{
				err:     fmt.Errorf("not authenticated"),
				harness: harness,
			}
		}

		// Check cache.
		if bundle.IsCached(namespace, slug, version) {
			return bundleCacheHitMsg{
				cachePath: bundle.CachePath(namespace, slug, version),
				harness:   harness,
			}
		}

		// Need to download — resolve first to get manifest.
		resolveCtx := context.Background()

		resolved, err := deps.Client.ResolveBundle(resolveCtx, namespace, slug, version)
		if err != nil {
			return bundleDownloadErrorMsg{
				err:     fmt.Errorf("resolve bundle: %w", err),
				harness: harness,
			}
		}

		// Download assets inline (returns progress messages via a channel would
		// be ideal, but for simplicity we do a blocking download and return completion).
		cachePath := bundle.CachePath(namespace, slug, resolved.Version)

		if err := downloadBundle(resolveCtx, deps.Client, resolved, cachePath); err != nil {
			return bundleDownloadErrorMsg{
				err:     err,
				harness: harness,
			}
		}

		return bundleDownloadCompleteMsg{
			cachePath: cachePath,
			harness:   harness,
		}
	}
}

// downloadBundle downloads all assets, verifies them, and writes the manifest.
func downloadBundle(ctx context.Context, c *client.Client, resolved *client.BundleResolveResponse, cachePath string) error {
	if len(resolved.Manifest.Layers) == 0 {
		return fmt.Errorf("bundle has no downloadable assets")
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return fmt.Errorf("create cache parent: %w", err)
	}

	// Create staging directory.
	stagingDir, err := os.MkdirTemp(filepath.Dir(cachePath), filepath.Base(cachePath)+".partial.")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}

	stagingFailed := true

	defer func() {
		if stagingFailed {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	assetsDir := filepath.Join(stagingDir, "assets")

	if mkdirErr := os.MkdirAll(assetsDir, 0o755); mkdirErr != nil { //nolint:gosec // G301: subdirs inside private cache root
		return fmt.Errorf("create staging assets directory: %w", mkdirErr)
	}

	// Download each asset.
	for _, layer := range resolved.Manifest.Layers {
		if validateErr := bundle.ValidateLogicalPath(layer.LogicalPath); validateErr != nil {
			return fmt.Errorf("invalid logical path %q: %w", layer.LogicalPath, validateErr)
		}

		if layer.AssetID == "" {
			return fmt.Errorf("asset %s is missing asset ID", layer.LogicalPath)
		}

		data, fetchErr := c.FetchBundleAsset(ctx, layer.AssetID)
		if fetchErr != nil {
			return fmt.Errorf("fetch asset %s: %w", layer.AssetID, fetchErr)
		}

		// Verify SHA256.
		if layer.ContentSHA256 != "" {
			h := sha256.Sum256(data)
			got := hex.EncodeToString(h[:])

			if got != layer.ContentSHA256 {
				return fmt.Errorf("asset %s: sha256 mismatch (got %s, want %s)", layer.LogicalPath, got, layer.ContentSHA256)
			}
		}

		// Write to staging.
		destPath := filepath.Join(assetsDir, layer.LogicalPath)
		if mkdirErr := os.MkdirAll(filepath.Dir(destPath), 0o755); mkdirErr != nil { //nolint:gosec // G301: cache subdirs
			return fmt.Errorf("create asset directory: %w", mkdirErr)
		}

		if writeErr := os.WriteFile(destPath, data, 0o644); writeErr != nil { //nolint:gosec // G306: cache files are readable
			return fmt.Errorf("write asset %s: %w", layer.LogicalPath, writeErr)
		}
	}

	// Write manifest (serves as cache-hit marker).
	manifestData, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Join(stagingDir, "manifest.json"), manifestData, 0o644); err != nil { //nolint:gosec // G306: manifest is readable
		return fmt.Errorf("write manifest: %w", err)
	}

	// Atomically promote staging to final cache path.
	if err := os.Rename(stagingDir, cachePath); err != nil {
		// Another process may have won the race — check if the final cache already exists.
		if _, statErr := os.Stat(filepath.Join(cachePath, "manifest.json")); statErr == nil {
			_ = os.RemoveAll(stagingDir)
			stagingFailed = false

			return nil
		}

		return fmt.Errorf("promote staging cache: %w", err)
	}

	stagingFailed = false

	return nil
}
