package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func clearStoreEnv(t *testing.T) {
	t.Helper()

	for _, env := range []string{
		"MUSHER_HOME", "MUSHER_CACHE_HOME",
		"XDG_CACHE_HOME",
	} {
		t.Setenv(env, "")
	}
}

func TestStoreBlob_WritesAndReads(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	data := []byte("hello world")

	digest, err := StoreBlob(data)
	if err != nil {
		t.Fatalf("StoreBlob() error = %v", err)
	}

	if digest == "" {
		t.Fatal("StoreBlob() returned empty digest")
	}

	// Read it back.
	got, err := ReadBlob(digest)
	if err != nil {
		t.Fatalf("ReadBlob() error = %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("ReadBlob() = %q, want %q", got, data)
	}
}

func TestStoreBlob_PrefixDir(t *testing.T) {
	clearStoreEnv(t)
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	data := []byte("prefix test")

	digest, err := StoreBlob(data)
	if err != nil {
		t.Fatalf("StoreBlob() error = %v", err)
	}

	// Verify the blob is stored in a 2-char prefix subdirectory.
	prefix := digest[:2]
	expectedPath := filepath.Join(cacheHome, "musher", "blobs", "sha256", prefix, digest)

	if _, statErr := os.Stat(expectedPath); statErr != nil {
		t.Fatalf("blob not found at expected prefix path %s: %v", expectedPath, statErr)
	}
}

func TestStoreBlob_Idempotent(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	data := []byte("idempotent content")

	d1, err1 := StoreBlob(data)
	d2, err2 := StoreBlob(data)

	if err1 != nil || err2 != nil {
		t.Fatalf("StoreBlob errors: %v, %v", err1, err2)
	}

	if d1 != d2 {
		t.Fatalf("StoreBlob digests differ: %q vs %q", d1, d2)
	}
}

func TestStoreAndLoadManifest(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	resolved := &client.BundleResolveResponse{
		BundleID:  "b1",
		Version:   "1.0.0",
		Namespace: "acme",
		Slug:      "test-bundle",
	}

	err := StoreManifest("api.musher.dev", "acme", "test-bundle", "1.0.0", resolved)
	if err != nil {
		t.Fatalf("StoreManifest() error = %v", err)
	}

	if !HasManifest("api.musher.dev", "acme", "test-bundle", "1.0.0") {
		t.Fatal("HasManifest() returned false after StoreManifest")
	}

	loaded, err := LoadManifest("api.musher.dev", "acme", "test-bundle", "1.0.0")
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if loaded.BundleID != "b1" || loaded.Version != "1.0.0" {
		t.Fatalf("LoadManifest() = %+v, want BundleID=b1 Version=1.0.0", loaded)
	}
}

func TestUpdateAndReadRef(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	err := UpdateRef("api.musher.dev", "acme", "test-bundle", "2.0.0")
	if err != nil {
		t.Fatalf("UpdateRef() error = %v", err)
	}

	ref, err := ReadRef("api.musher.dev", "acme", "test-bundle")
	if err != nil {
		t.Fatalf("ReadRef() error = %v", err)
	}

	if ref.Version != "2.0.0" {
		t.Fatalf("ReadRef().Version = %q, want %q", ref.Version, "2.0.0")
	}

	if !IsRefFresh(ref) {
		t.Fatal("ReadRef() should return a fresh ref")
	}
}

func TestEnsureCacheDirTag(t *testing.T) {
	clearStoreEnv(t)

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	EnsureCacheDirTag()

	tagPath := filepath.Join(cacheHome, "musher", "CACHEDIR.TAG")

	data, err := os.ReadFile(tagPath)
	if err != nil {
		t.Fatalf("CACHEDIR.TAG not created: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("CACHEDIR.TAG is empty")
	}
}

func TestPruneBlobs_RemovesUnreferenced(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Store two blobs.
	d1, _ := StoreBlob([]byte("referenced"))
	_, _ = StoreBlob([]byte("unreferenced"))

	// Store a manifest that references only d1.
	resolved := &client.BundleResolveResponse{
		BundleID:  "b1",
		Version:   "1.0.0",
		Namespace: "acme",
		Slug:      "test",
		Manifest: client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "file.txt", ContentSHA256: d1},
			},
		},
	}

	if err := StoreManifest("api.musher.dev", "acme", "test", "1.0.0", resolved); err != nil {
		t.Fatalf("StoreManifest() error = %v", err)
	}

	pruned, err := PruneBlobs()
	if err != nil {
		t.Fatalf("PruneBlobs() error = %v", err)
	}

	if pruned != 1 {
		t.Fatalf("PruneBlobs() pruned = %d, want 1", pruned)
	}

	// Referenced blob should still exist.
	if _, err := ReadBlob(d1); err != nil {
		t.Fatalf("referenced blob missing after prune: %v", err)
	}
}

func TestReadAsset(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	data := []byte("asset content")

	digest, err := StoreBlob(data)
	if err != nil {
		t.Fatalf("StoreBlob() error = %v", err)
	}

	layer := &client.BundleLayer{
		LogicalPath:   "skills/test/SKILL.md",
		ContentSHA256: digest,
	}

	got, err := ReadAsset(layer)
	if err != nil {
		t.Fatalf("ReadAsset() error = %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Fatalf("ReadAsset() = %q, want %q", got, data)
	}
}

func TestReadAsset_NoDigest(t *testing.T) {
	layer := &client.BundleLayer{
		LogicalPath: "skills/test/SKILL.md",
	}

	_, err := ReadAsset(layer)
	if err == nil {
		t.Fatal("ReadAsset() expected error for empty digest")
	}
}

func TestHasAllBlobs(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	d1, _ := StoreBlob([]byte("blob1"))
	d2, _ := StoreBlob([]byte("blob2"))

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "a.txt", ContentSHA256: d1},
			{LogicalPath: "b.txt", ContentSHA256: d2},
		},
	}

	if !HasAllBlobs(manifest) {
		t.Fatal("HasAllBlobs() should be true when all blobs exist")
	}

	manifest.Layers = append(manifest.Layers, client.BundleLayer{
		LogicalPath:   "c.txt",
		ContentSHA256: "nonexistent",
	})

	if HasAllBlobs(manifest) {
		t.Fatal("HasAllBlobs() should be false when a blob is missing")
	}
}

func TestClearAll(t *testing.T) {
	clearStoreEnv(t)
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	_, _ = StoreBlob([]byte("data"))

	err := ClearAll()
	if err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}

	cacheDir := filepath.Join(cacheHome, "musher")
	if _, statErr := os.Stat(cacheDir); !os.IsNotExist(statErr) {
		t.Fatalf("ClearAll() did not remove cache dir")
	}
}

func TestPurgeBundle(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	resolved := &client.BundleResolveResponse{
		BundleID:  "b1",
		Version:   "1.0.0",
		Namespace: "acme",
		Slug:      "test",
	}

	_ = StoreManifest("host", "acme", "test", "1.0.0", resolved)
	_ = UpdateRef("host", "acme", "test", "1.0.0")

	if !HasManifest("host", "acme", "test", "1.0.0") {
		t.Fatal("manifest should exist before purge")
	}

	err := PurgeBundle("host", "acme", "test")
	if err != nil {
		t.Fatalf("PurgeBundle() error = %v", err)
	}

	if HasManifest("host", "acme", "test", "1.0.0") {
		t.Fatal("manifest should not exist after purge")
	}
}

func TestIsManifestFresh(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	resolved := &client.BundleResolveResponse{
		BundleID:  "b1",
		Version:   "1.0.0",
		Namespace: "acme",
		Slug:      "test",
	}

	_ = StoreManifest("host", "acme", "test", "1.0.0", resolved)
	_ = StoreManifestMeta("host", "acme", "test", "1.0.0", &ManifestMeta{
		FetchedAt: time.Now(),
		TTL:       3600,
	})

	if !IsManifestFresh("host", "acme", "test", "1.0.0") {
		t.Fatal("IsManifestFresh() should be true for recently fetched manifest")
	}

	// Store with TTL=0 (already expired).
	_ = StoreManifestMeta("host", "acme", "test", "1.0.0", &ManifestMeta{
		FetchedAt: time.Now().Add(-time.Hour),
		TTL:       0,
	})

	if IsManifestFresh("host", "acme", "test", "1.0.0") {
		t.Fatal("IsManifestFresh() should be false for expired manifest")
	}
}
