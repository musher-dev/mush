package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSampleBundle(t *testing.T) {
	resolved, cachePath, cleanup, err := ExtractSampleBundle()
	if err != nil {
		t.Fatalf("ExtractSampleBundle() error = %v", err)
	}

	defer cleanup()

	if resolved.Namespace != "_local" {
		t.Errorf("Namespace = %q, want %q", resolved.Namespace, "_local")
	}

	if resolved.Slug != "sample" {
		t.Errorf("Slug = %q, want %q", resolved.Slug, "sample")
	}

	if resolved.Version != "0.0.0-sample" {
		t.Errorf("Version = %q, want %q", resolved.Version, "0.0.0-sample")
	}

	if len(resolved.Manifest.Layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(resolved.Manifest.Layers))
	}

	layer := resolved.Manifest.Layers[0]
	if layer.AssetType != "skill" {
		t.Errorf("AssetType = %q, want %q", layer.AssetType, "skill")
	}

	if layer.LogicalPath != "skills/hello/SKILL.md" {
		t.Errorf("LogicalPath = %q, want %q", layer.LogicalPath, "skills/hello/SKILL.md")
	}

	if layer.SizeBytes <= 0 {
		t.Error("SizeBytes should be positive")
	}

	if layer.ContentSHA256 == "" {
		t.Error("ContentSHA256 should be set")
	}

	// Verify cache structure.
	assetPath := filepath.Join(cachePath, "assets", "skills", "hello", "SKILL.md")
	if _, err := os.Stat(assetPath); err != nil {
		t.Errorf("asset not found at %s: %v", assetPath, err)
	}

	manifestPath := filepath.Join(cachePath, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("manifest.json not found: %v", err)
	}

	// Verify cleanup works.
	cleanup()

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove temp dir %s", cachePath)
	}
}
