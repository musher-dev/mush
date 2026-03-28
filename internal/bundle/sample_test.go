package bundle

import (
	"testing"
)

func TestExtractSampleBundle(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	resolved, err := ExtractSampleBundle()
	if err != nil {
		t.Fatalf("ExtractSampleBundle() error = %v", err)
	}

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

	// Verify asset is readable via ReadAsset.
	data, readErr := ReadAsset(&layer)
	if readErr != nil {
		t.Fatalf("ReadAsset() error = %v", readErr)
	}

	if len(data) == 0 {
		t.Fatal("ReadAsset() returned empty data")
	}
}
