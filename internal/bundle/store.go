package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/paths"
	"github.com/musher-dev/mush/internal/safeio"
)

// Content-addressable cache layout under {CacheRoot}/:
//
//	blobs/sha256/{digest}                              — raw asset content
//	manifests/{hostID}/{namespace}/{slug}/{version}.json — resolved bundle manifest
//	refs/{hostID}/{namespace}/{slug}/latest             — latest version pointer
//	bundles/{namespace}/{slug}/{version}/               — materialized view
//	CACHEDIR.TAG                                        — standard cache exclusion marker

const (
	cachedirTagContent = "Signature: 8a477f597d28d172789f06886806bc55\n" +
		"# This file is a cache directory tag created by mush.\n" +
		"# For information about cache directory tags, see:\n" +
		"#   https://bford.info/cachedir/spec.html\n"
)

// cacheRootDir returns the top-level cache root (parent of bundles/).
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

	blobDir := filepath.Join(cacheRootDir(), "blobs", "sha256")
	blobPath := filepath.Join(blobDir, digest)

	if _, err := os.Stat(blobPath); err == nil {
		return digest, nil // already stored
	}

	if err := safeio.MkdirAll(blobDir, 0o700); err != nil {
		return "", fmt.Errorf("create blob directory: %w", err)
	}

	if err := safeio.WriteFile(blobPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write blob %s: %w", digest, err)
	}

	return digest, nil
}

// ReadBlob reads a blob by its SHA256 digest.
func ReadBlob(digest string) ([]byte, error) {
	blobPath := filepath.Join(cacheRootDir(), "blobs", "sha256", digest)

	data, err := safeio.ReadFile(blobPath)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", digest, err)
	}

	return data, nil
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

// UpdateRef writes a latest-version pointer for a bundle.
func UpdateRef(hostID, namespace, slug, version string) error {
	refDir := filepath.Join(cacheRootDir(), "refs", hostID, namespace, slug)
	if err := safeio.MkdirAll(refDir, 0o700); err != nil {
		return fmt.Errorf("create ref directory: %w", err)
	}

	refPath := filepath.Join(refDir, "latest")

	if err := safeio.WriteFile(refPath, []byte(version+"\n"), 0o644); err != nil {
		return fmt.Errorf("write ref: %w", err)
	}

	return nil
}

// ReadRef reads the latest-version pointer for a bundle.
func ReadRef(hostID, namespace, slug string) (string, error) {
	refPath := filepath.Join(cacheRootDir(), "refs", hostID, namespace, slug, "latest")

	data, err := safeio.ReadFile(refPath)
	if err != nil {
		return "", fmt.Errorf("read ref: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// PruneBlobs removes blobs that are not referenced by any cached manifest.
func PruneBlobs() (int, error) {
	blobDir := filepath.Join(cacheRootDir(), "blobs", "sha256")

	entries, err := os.ReadDir(blobDir)
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

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !referenced[entry.Name()] {
			if removeErr := os.Remove(filepath.Join(blobDir, entry.Name())); removeErr == nil {
				pruned++
			}
		}
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

		if dirEntry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		data, readErr := safeio.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable manifests
		}

		var manifest client.BundleResolveResponse
		if jsonErr := json.Unmarshal(data, &manifest); jsonErr != nil {
			return nil //nolint:nilerr // skip corrupt manifests
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

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}
