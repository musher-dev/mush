package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
)

func TestLoadFromDir_CacheCompatible(t *testing.T) {
	// Create a directory with assets/ subdirectory (cache-compatible layout).
	dir := t.TempDir()
	assetsDir := filepath.Join(dir, "assets", "skills", "hello")

	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := []byte("# Hello Skill\n")
	if err := os.WriteFile(filepath.Join(assetsDir, "SKILL.md"), skillContent, 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, cachePath, cleanup, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	defer cleanup()

	if resolved.Namespace != "_local" {
		t.Errorf("Namespace = %q, want %q", resolved.Namespace, "_local")
	}

	if resolved.Version != "0.0.0-local" {
		t.Errorf("Version = %q, want %q", resolved.Version, "0.0.0-local")
	}

	if cachePath != dir {
		t.Errorf("cachePath = %q, want %q", cachePath, dir)
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

	// Verify manifest.json was written.
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Errorf("manifest.json not written: %v", err)
	}
}

func TestLoadFromDir_CacheCompatibleWithManifest(t *testing.T) {
	// Create a directory with assets/ and pre-existing manifest.json.
	dir := t.TempDir()
	assetsDir := filepath.Join(dir, "assets", "skills", "hello")

	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(assetsDir, "SKILL.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a manifest with custom fields.
	manifest := &client.BundleResolveResponse{
		Namespace: "_local",
		Slug:      "pre-existing",
		Version:   "1.2.3",
		Manifest: client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "skills/hello/SKILL.md", AssetType: "skill"},
			},
		},
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, _, cleanup, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	defer cleanup()

	// Should use pre-existing manifest values.
	if resolved.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q (from existing manifest)", resolved.Version, "1.2.3")
	}

	if resolved.Slug != "pre-existing" {
		t.Errorf("Slug = %q, want %q", resolved.Slug, "pre-existing")
	}
}

func TestLoadFromDir_BareDirectory(t *testing.T) {
	// Create a bare directory without assets/ subdirectory.
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills", "greet")

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# Greet\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, cachePath, cleanup, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	defer cleanup()

	if resolved.Namespace != "_local" {
		t.Errorf("Namespace = %q, want %q", resolved.Namespace, "_local")
	}

	// cachePath should be a temp dir, not the original dir.
	if cachePath == dir {
		t.Error("cachePath should be a temp dir for bare layout, got original dir")
	}

	// Verify the symlinked asset exists in the cache structure.
	assetPath := filepath.Join(cachePath, "assets", "skills", "greet", "SKILL.md")
	if _, lstatErr := os.Lstat(assetPath); lstatErr != nil {
		t.Errorf("symlinked asset not found: %v", lstatErr)
	}

	// Verify the symlink target.
	target, linkErr := os.Readlink(assetPath)
	if linkErr != nil {
		t.Errorf("readlink failed: %v", linkErr)
	}

	expectedTarget := filepath.Join(dir, "skills", "greet", "SKILL.md")
	if target != expectedTarget {
		t.Errorf("symlink target = %q, want %q", target, expectedTarget)
	}

	if len(resolved.Manifest.Layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(resolved.Manifest.Layers))
	}

	// Cleanup should remove the temp dir.
	cleanup()

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove temp dir %s", cachePath)
	}
}

func TestLoadFromDir_MissingDir(t *testing.T) {
	_, _, _, err := LoadFromDir("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestLoadFromDir_NotADirectory(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "mush-test-*")
	if err != nil {
		t.Fatal(err)
	}

	f.Close()

	_, _, _, err = LoadFromDir(f.Name())
	if err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestLoadFromDir_EmptyAssetsDir(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected error for empty assets directory")
	}
}

func TestLoadFromDir_EmptyBareDir(t *testing.T) {
	dir := t.TempDir()

	// Write a non-bundle file.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected error for directory with no recognized assets")
	}
}

func TestInferAssetType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"skills/hello/SKILL.md", "skill"},
		{"skills/deep/nested/SKILL.md", "skill"},
		{"SKILL.md", "skill"},
		{"agents/my-agent/AGENT.md", "agent_definition"},
		{"AGENT.md", "agent_definition"},
		{".mcp.json", "tool_config"},
		{"mcp.json", "tool_config"},
		{"tools/server.toml", "tool_config"},
		{"tools/config.json", "tool_config"},
		{"README.md", ""},
		{"src/main.go", ""},
		{"random.txt", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := inferAssetType(tt.path)
			if got != tt.want {
				t.Errorf("inferAssetType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
