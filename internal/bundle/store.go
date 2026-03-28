package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/paths"
	"github.com/musher-dev/mush/internal/safeio"
)

// Content-addressable cache layout under {CacheRoot}/:
//
//	blobs/sha256/{prefix}/{digest}                      — raw asset content (2-char prefix subdirs)
//	manifests/{hostID}/{namespace}/{slug}/{version}.json — resolved bundle manifest
//	manifests/{hostID}/{namespace}/{slug}/{version}.meta.json — manifest metadata sidecar
//	refs/{hostID}/{namespace}/{slug}/latest.json         — latest version pointer (JSON with TTL)
//	CACHEDIR.TAG                                         — standard cache exclusion marker

const (
	cachedirTagContent = "Signature: 8a477f597d28d172789f06886806bc55\n" +
		"# This file is a cache directory tag created by mush.\n" +
		"# For information about cache directory tags, see:\n" +
		"#   https://bford.info/cachedir/spec.html\n"

	// DefaultManifestTTL is the default manifest freshness TTL in seconds (24 hours).
	DefaultManifestTTL = 86400

	// DefaultRefTTL is the default ref freshness TTL in seconds (5 minutes).
	DefaultRefTTL = 300
)

// ManifestMeta holds metadata about a cached manifest.
type ManifestMeta struct {
	FetchedAt time.Time `json:"fetchedAt"`
	TTL       int       `json:"ttl"` // seconds; 86400 = 24h
	OCIDigest string    `json:"ociDigest,omitempty"`
}

// RefData holds a versioned ref pointer with TTL.
type RefData struct {
	Version   string    `json:"version"`
	CachedAt  time.Time `json:"cachedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// cacheRootDir returns the top-level cache root.
func cacheRootDir() string {
	root, err := paths.CacheRoot()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(os.TempDir(), "musher")
		}

		return filepath.Join(home, ".cache", "musher")
	}

	return root
}

// CacheRootDir returns the top-level cache root directory (exported for cache management commands).
func CacheRootDir() string {
	return cacheRootDir()
}

// blobPath returns the path for a blob with 2-char prefix subdirectory.
func blobPath(digest string) string {
	return filepath.Join(cacheRootDir(), "blobs", "sha256", digest[:2], digest)
}

// EnsureCacheDirTag writes a CACHEDIR.TAG file in the cache root if it doesn't exist.
func EnsureCacheDirTag() {
	root := cacheRootDir()

	tagPath := filepath.Join(root, "CACHEDIR.TAG")
	if _, err := os.Stat(tagPath); err == nil {
		return
	}

	if err := safeio.MkdirAll(root, 0o700); err != nil {
		return
	}

	_ = safeio.WriteFile(tagPath, []byte(cachedirTagContent), 0o644)
}

// StoreBlob writes content to the blob store keyed by SHA256 digest.
// Returns the hex-encoded digest. If the blob already exists, this is a no-op.
func StoreBlob(data []byte) (string, error) {
	digest := sha256Hex(data)
	blobFilePath := blobPath(digest)

	if _, err := os.Stat(blobFilePath); err == nil {
		return digest, nil // already stored
	}

	if err := safeio.MkdirAll(filepath.Dir(blobFilePath), 0o700); err != nil {
		return "", fmt.Errorf("create blob directory: %w", err)
	}

	if err := safeio.WriteFile(blobFilePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write blob %s: %w", digest, err)
	}

	return digest, nil
}

// ReadBlob reads a blob by its SHA256 digest.
func ReadBlob(digest string) ([]byte, error) {
	blobFilePath := blobPath(digest)

	data, err := safeio.ReadFile(blobFilePath)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", digest, err)
	}

	return data, nil
}

// ReadAsset reads an asset from the blob store using the layer's content digest.
func ReadAsset(layer *client.BundleLayer) ([]byte, error) {
	if layer.ContentSHA256 == "" {
		return nil, fmt.Errorf("layer %s has no content digest", layer.LogicalPath)
	}

	return ReadBlob(layer.ContentSHA256)
}

// HasBlob checks if a blob exists in the store.
func HasBlob(digest string) bool {
	_, err := os.Stat(blobPath(digest))

	return err == nil
}

// HasAllBlobs checks if all blobs referenced by a manifest exist in the store.
func HasAllBlobs(manifest *client.BundleManifest) bool {
	if manifest == nil {
		return true
	}

	for _, layer := range manifest.Layers {
		if layer.ContentSHA256 != "" && !HasBlob(layer.ContentSHA256) {
			return false
		}
	}

	return true
}

// StoreManifest writes a resolved bundle manifest to the manifest store.
func StoreManifest(hostID, namespace, slug, version string, resolved *client.BundleResolveResponse) error {
	manifestDir := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug)
	if err := safeio.MkdirAll(manifestDir, 0o700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(manifestDir, version+".json")
	if err := safeio.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	return nil
}

// LoadManifest reads a cached manifest for a specific bundle version.
func LoadManifest(hostID, namespace, slug, version string) (*client.BundleResolveResponse, error) {
	manifestPath := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug, version+".json")

	data, err := safeio.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var resolved client.BundleResolveResponse
	if err := json.Unmarshal(data, &resolved); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	return &resolved, nil
}

// HasManifest checks if a manifest exists for a given bundle version.
func HasManifest(hostID, namespace, slug, version string) bool {
	manifestPath := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug, version+".json")
	_, err := os.Stat(manifestPath)

	return err == nil
}

// StoreManifestMeta writes a manifest metadata sidecar.
func StoreManifestMeta(hostID, namespace, slug, version string, meta *ManifestMeta) error {
	manifestDir := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug)
	if err := safeio.MkdirAll(manifestDir, 0o700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest meta: %w", err)
	}

	metaPath := filepath.Join(manifestDir, version+".meta.json")
	if err := safeio.WriteFile(metaPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest meta: %w", err)
	}

	return nil
}

// LoadManifestMeta reads a manifest metadata sidecar.
func LoadManifestMeta(hostID, namespace, slug, version string) (*ManifestMeta, error) {
	metaPath := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug, version+".meta.json")

	data, err := safeio.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest meta: %w", err)
	}

	var meta ManifestMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal manifest meta: %w", err)
	}

	return &meta, nil
}

// IsManifestFresh checks if a cached manifest is still fresh based on its metadata TTL.
func IsManifestFresh(hostID, namespace, slug, version string) bool {
	meta, err := LoadManifestMeta(hostID, namespace, slug, version)
	if err != nil {
		return false
	}

	return meta.FetchedAt.Add(time.Duration(meta.TTL) * time.Second).After(time.Now())
}

// UpdateRef writes a latest-version pointer for a bundle as JSON with TTL.
func UpdateRef(hostID, namespace, slug, version string) error {
	refDir := filepath.Join(cacheRootDir(), "refs", hostID, namespace, slug)
	if err := safeio.MkdirAll(refDir, 0o700); err != nil {
		return fmt.Errorf("create ref directory: %w", err)
	}

	now := time.Now()
	ref := RefData{
		Version:   version,
		CachedAt:  now,
		ExpiresAt: now.Add(time.Duration(DefaultRefTTL) * time.Second),
	}

	data, err := json.MarshalIndent(&ref, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ref: %w", err)
	}

	refPath := filepath.Join(refDir, "latest.json")

	if err := safeio.WriteFile(refPath, data, 0o644); err != nil {
		return fmt.Errorf("write ref: %w", err)
	}

	return nil
}

// ReadRef reads the latest-version pointer for a bundle.
func ReadRef(hostID, namespace, slug string) (*RefData, error) {
	refPath := filepath.Join(cacheRootDir(), "refs", hostID, namespace, slug, "latest.json")

	data, err := safeio.ReadFile(refPath)
	if err != nil {
		return nil, fmt.Errorf("read ref: %w", err)
	}

	var ref RefData
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("unmarshal ref: %w", err)
	}

	return &ref, nil
}

// IsRefFresh checks if a ref pointer is still fresh.
func IsRefFresh(ref *RefData) bool {
	if ref == nil {
		return false
	}

	return ref.ExpiresAt.After(time.Now())
}

// PruneBlobs removes blobs that are not referenced by any cached manifest.
func PruneBlobs() (int, error) {
	blobRoot := filepath.Join(cacheRootDir(), "blobs", "sha256")

	prefixEntries, err := os.ReadDir(blobRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("read blob directory: %w", err)
	}

	// Collect all referenced digests from manifests.
	referenced, walkErr := collectReferencedDigests()
	if walkErr != nil {
		return 0, fmt.Errorf("collect referenced digests: %w", walkErr)
	}

	pruned := 0

	for _, prefixEntry := range prefixEntries {
		if !prefixEntry.IsDir() {
			continue
		}

		prunedInDir, dirErr := prunePrefixDir(filepath.Join(blobRoot, prefixEntry.Name()), referenced)
		if dirErr != nil {
			continue
		}

		pruned += prunedInDir
	}

	return pruned, nil
}

// prunePrefixDir prunes unreferenced blobs from a single 2-char prefix directory
// and removes the directory if it becomes empty.
func prunePrefixDir(prefixDir string, referenced map[string]bool) (int, error) {
	entries, err := os.ReadDir(prefixDir)
	if err != nil {
		return 0, fmt.Errorf("read prefix dir %s: %w", prefixDir, err)
	}

	pruned := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !referenced[entry.Name()] {
			if removeErr := os.Remove(filepath.Join(prefixDir, entry.Name())); removeErr == nil {
				pruned++
			}
		}
	}

	// Remove empty prefix directory.
	remaining, _ := os.ReadDir(prefixDir)
	if len(remaining) == 0 {
		_ = os.Remove(prefixDir)
	}

	return pruned, nil
}

// collectReferencedDigests walks all manifests and returns the set of blob digests they reference.
// Returns an error if the manifests directory cannot be walked, to prevent PruneBlobs from
// deleting all blobs when the manifest directory is unreadable.
func collectReferencedDigests() (map[string]bool, error) {
	digests := make(map[string]bool)

	manifestRoot := filepath.Join(cacheRootDir(), "manifests")

	if _, err := os.Stat(manifestRoot); os.IsNotExist(err) {
		return digests, nil
	}

	walkErr := filepath.WalkDir(manifestRoot, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if dirEntry.IsDir() || !strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		data, readErr := safeio.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable manifests during digest collection
		}

		var manifest client.BundleResolveResponse
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			return nil //nolint:nilerr // skip corrupt manifests during digest collection
		}

		for _, layer := range manifest.Manifest.Layers {
			if layer.ContentSHA256 != "" {
				digests[layer.ContentSHA256] = true
			}
		}

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk manifests: %w", walkErr)
	}

	return digests, nil
}

// CleanExpired removes expired manifests and refs, then garbage-collects unreferenced blobs.
func CleanExpired() (manifests, refs, blobs int, err error) {
	manifests, err = cleanExpiredManifests()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("clean expired manifests: %w", err)
	}

	refs, err = cleanExpiredRefs()
	if err != nil {
		return manifests, 0, 0, fmt.Errorf("clean expired refs: %w", err)
	}

	blobs, err = PruneBlobs()
	if err != nil {
		return manifests, refs, 0, fmt.Errorf("prune blobs: %w", err)
	}

	return manifests, refs, blobs, nil
}

// cleanExpiredManifests removes manifest + meta pairs whose TTL has expired.
func cleanExpiredManifests() (int, error) {
	manifestRoot := filepath.Join(cacheRootDir(), "manifests")

	if _, err := os.Stat(manifestRoot); os.IsNotExist(err) {
		return 0, nil
	}

	removed := 0

	walkErr := filepath.WalkDir(manifestRoot, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if dirEntry.IsDir() || !strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		data, readErr := safeio.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable meta files during cleanup
		}

		var meta ManifestMeta
		if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
			return nil //nolint:nilerr // skip corrupt meta files during cleanup
		}

		if meta.FetchedAt.Add(time.Duration(meta.TTL) * time.Second).Before(time.Now()) {
			// Remove both meta and manifest.
			manifestPath := strings.TrimSuffix(path, ".meta.json") + ".json"
			_ = os.Remove(path)
			_ = os.Remove(manifestPath)
			removed++
		}

		return nil
	})
	if walkErr != nil {
		return removed, fmt.Errorf("walk manifests for cleanup: %w", walkErr)
	}

	return removed, nil
}

// cleanExpiredRefs removes ref files whose TTL has expired.
func cleanExpiredRefs() (int, error) {
	refsRoot := filepath.Join(cacheRootDir(), "refs")

	if _, err := os.Stat(refsRoot); os.IsNotExist(err) {
		return 0, nil
	}

	removed := 0

	walkErr := filepath.WalkDir(refsRoot, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if dirEntry.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, readErr := safeio.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable ref files during cleanup
		}

		var ref RefData
		if jsonErr := json.Unmarshal(data, &ref); jsonErr != nil {
			return nil //nolint:nilerr // skip corrupt ref files during cleanup
		}

		if ref.ExpiresAt.Before(time.Now()) {
			_ = os.Remove(path)
			removed++
		}

		return nil
	})
	if walkErr != nil {
		return removed, fmt.Errorf("walk refs for cleanup: %w", walkErr)
	}

	return removed, nil
}

// PurgeBundle removes all cached data for a specific bundle (manifests, refs).
func PurgeBundle(hostID, namespace, slug string) error {
	manifestDir := filepath.Join(cacheRootDir(), "manifests", hostID, namespace, slug)
	if err := os.RemoveAll(manifestDir); err != nil {
		return fmt.Errorf("remove manifest dir: %w", err)
	}

	refDir := filepath.Join(cacheRootDir(), "refs", hostID, namespace, slug)
	if err := os.RemoveAll(refDir); err != nil {
		return fmt.Errorf("remove ref dir: %w", err)
	}

	return nil
}

// ClearAll removes the entire cache directory.
func ClearAll() error {
	root := cacheRootDir()
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("remove cache root: %w", err)
	}

	return nil
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}
