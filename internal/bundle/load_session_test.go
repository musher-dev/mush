package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestPrepareLoadSession_AddDirHarnessDoesNotTouchProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := t.TempDir()

	writeCachedAsset(t, cacheDir, "skills/web/SKILL.md", "skill")
	writeCachedAsset(t, cacheDir, "researcher.md", "agent")
	writeCachedAsset(t, cacheDir, "mcp.json", `{"mcpServers":{}}`)

	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	session, err := PrepareLoadSession(
		t.Context(),
		projectDir,
		cacheDir,
		&client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
				{LogicalPath: "researcher.md", AssetType: "agent_definition"},
				{LogicalPath: "mcp.json", AssetType: "tool_config"},
			},
		},
		spec,
		NewProviderMapper(spec),
	)
	if err != nil {
		t.Fatalf("PrepareLoadSession() error = %v", err)
	}

	defer session.Cleanup()

	if session.BundleDir == projectDir {
		t.Fatal("add_dir harness should use an external bundle directory")
	}

	if session.WorkingDir != projectDir {
		t.Fatalf("WorkingDir = %q, want %q", session.WorkingDir, projectDir)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("project dir should not be mutated for add_dir harness; stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(session.BundleDir, ".claude", "skills", "web", "SKILL.md")); err != nil {
		t.Fatalf("expected skill in external bundle dir: %v", err)
	}
}

func TestPrepareLoadSession_CWDHarnessFallsBackToProjectInjection(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := t.TempDir()

	writeCachedAsset(t, cacheDir, "commands/review.toml", "prompt = \"review\"")
	writeCachedAsset(t, cacheDir, "tools/settings.json", `{"theme":"test"}`)

	spec, ok := harness.GetProvider("gemini")
	if !ok {
		t.Fatal("gemini provider not found")
	}

	session, err := PrepareLoadSession(
		t.Context(),
		projectDir,
		cacheDir,
		&client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "commands/review.toml", AssetType: "skill"},
				{LogicalPath: "tools/settings.json", AssetType: "tool_config"},
			},
		},
		spec,
		NewProviderMapper(spec),
	)
	if err != nil {
		t.Fatalf("PrepareLoadSession() error = %v", err)
	}

	defer session.Cleanup()

	if session.BundleDir != projectDir {
		t.Fatalf("BundleDir = %q, want project dir %q", session.BundleDir, projectDir)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".gemini", "commands", "review.toml")); err != nil {
		t.Fatalf("expected injected skill in project dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".gemini", "settings.json")); err != nil {
		t.Fatalf("expected injected tool config in project dir: %v", err)
	}
}

func writeCachedAsset(t *testing.T, cacheDir, rel, data string) {
	t.Helper()

	path := filepath.Join(cacheDir, "assets", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", rel, err)
	}

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", rel, err)
	}
}
