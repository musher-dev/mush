package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
)

func TestLoadFromDir_CacheCompatible(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

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

	resolved, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	if resolved.Namespace != "_local" {
		t.Errorf("Namespace = %q, want %q", resolved.Namespace, "_local")
	}

	if resolved.Version != "0.0.0-local" {
		t.Errorf("Version = %q, want %q", resolved.Version, "0.0.0-local")
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

	// Verify blob was stored and is readable via ReadAsset.
	data, readErr := ReadAsset(&layer)
	if readErr != nil {
		t.Fatalf("ReadAsset() error = %v", readErr)
	}

	if !bytes.Equal(data, skillContent) {
		t.Fatalf("ReadAsset() = %q, want %q", data, skillContent)
	}
}

func TestLoadFromDir_CacheCompatibleWithManifest(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Create a directory with assets/ and pre-existing manifest.json.
	dir := t.TempDir()
	assetsDir := filepath.Join(dir, "assets", "skills", "hello")

	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := []byte("# Hello\n")
	if err := os.WriteFile(filepath.Join(assetsDir, "SKILL.md"), skillContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a manifest with custom fields.
	manifest := &client.BundleResolveResponse{
		Namespace: "_local",
		Slug:      "pre-existing",
		Version:   "1.2.3",
		Manifest: client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "skills/hello/SKILL.md", AssetType: "skill", ContentSHA256: sha256Hex(skillContent)},
			},
		},
	}

	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	// Should use pre-existing manifest values.
	if resolved.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q (from existing manifest)", resolved.Version, "1.2.3")
	}

	if resolved.Slug != "pre-existing" {
		t.Errorf("Slug = %q, want %q", resolved.Slug, "pre-existing")
	}
}

func TestLoadFromDir_BareDirectory(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Create a bare directory without assets/ subdirectory.
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills", "greet")

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := []byte("# Greet\n")
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), skillContent, 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}

	if resolved.Namespace != "_local" {
		t.Errorf("Namespace = %q, want %q", resolved.Namespace, "_local")
	}

	if len(resolved.Manifest.Layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(resolved.Manifest.Layers))
	}

	// Verify the asset is readable via ReadAsset.
	layer := resolved.Manifest.Layers[0]

	data, readErr := ReadAsset(&layer)
	if readErr != nil {
		t.Fatalf("ReadAsset() error = %v", readErr)
	}

	if !bytes.Equal(data, skillContent) {
		t.Fatalf("ReadAsset() = %q, want %q", data, skillContent)
	}
}

func TestLoadFromDir_MissingDir(t *testing.T) {
	_, err := LoadFromDir("/nonexistent/path")
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

	_, err = LoadFromDir(f.Name())
	if err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestLoadFromDir_EmptyAssetsDir(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected error for empty assets directory")
	}
}

func TestLoadFromDir_EmptyBareDir(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir := t.TempDir()

	// Write a non-bundle file.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFromDir(dir)
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
