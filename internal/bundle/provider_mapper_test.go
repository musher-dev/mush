package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestProviderMapper_NoAssets(t *testing.T) {
	spec, ok := harness.GetProvider("bash")
	if !ok {
		t.Fatal("bash provider not found")
	}

	mapper := NewProviderMapper(spec)

	_, err := mapper.MapAsset("/project", client.BundleLayer{AssetType: "skill", LogicalPath: "test.md"})
	if err == nil {
		t.Fatal("expected error for provider without asset support")
	}
}

func TestProviderMapper_PrepareLoad_Claude(t *testing.T) {
	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(spec)
	cacheDir := t.TempDir()

	assetsDir := filepath.Join(cacheDir, "assets")

	write := func(rel, data string) {
		path := filepath.Join(assetsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}

		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}

	write("skills/web/SKILL.md", "skill content")
	write("researcher.md", "agent content")
	write("mcp.json", `{"mcpServers":{}}`)

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
			{LogicalPath: "researcher.md", AssetType: "agent_definition"},
			{LogicalPath: "mcp.json", AssetType: "tool_config"},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), cacheDir, manifest)
	if err != nil {
		t.Fatalf("PrepareLoad error = %v", err)
	}

	defer cleanup()

	// Claude uses add_dir mode: discoverable assets (skills, agents) should be
	// excluded from tmpDir to avoid duplication with CWD injection.
	agentPath := filepath.Join(tmpDir, ".claude", "agents", "researcher.md")
	if _, statErr := os.Stat(agentPath); statErr == nil {
		t.Fatal("agent_definition should NOT be in tmpDir for add_dir mode harness")
	}

	skillPath := filepath.Join(tmpDir, ".claude", "skills", "skills", "web", "SKILL.md")
	if _, statErr := os.Stat(skillPath); statErr == nil {
		t.Fatal("skill should NOT be in tmpDir for add_dir mode harness")
	}

	// tool_config should still be present in tmpDir.
	toolPath := filepath.Join(tmpDir, ".mcp.json")

	data, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("ReadFile(tool_config) error = %v; tool_config should remain in tmpDir", err)
	}

	if len(data) == 0 {
		t.Fatal("tool_config file is empty")
	}
}

func TestProviderMapper_PrepareLoad_Codex(t *testing.T) {
	spec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(spec)
	cacheDir := t.TempDir()

	assetsDir := filepath.Join(cacheDir, "assets")

	write := func(rel, data string) {
		path := filepath.Join(assetsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}

		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}

	write("researcher.md", "Agent A")
	write("reviewer.md", "Agent B")
	write("tools/mcp.toml", "[mcp_servers.test]\ncommand = \"test\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "researcher.md", AssetType: "agent_definition"},
			{LogicalPath: "reviewer.md", AssetType: "agent_definition"},
			{LogicalPath: "tools/mcp.toml", AssetType: "tool_config"},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), cacheDir, manifest)
	if err != nil {
		t.Fatalf("PrepareLoad error = %v", err)
	}

	defer cleanup()

	// Codex now uses add_dir mode: discoverable assets (agents) should be
	// excluded from tmpDir to avoid duplication with CWD injection.
	for _, name := range []string{"researcher.md", "reviewer.md"} {
		agentPath := filepath.Join(tmpDir, ".codex", "agents", name)
		if _, statErr := os.Stat(agentPath); statErr == nil {
			t.Fatalf("agent_definition %s should NOT be in tmpDir for add_dir mode harness", name)
		}
	}

	// tool_config should still be present in tmpDir.
	toolPath := filepath.Join(tmpDir, ".codex", "config.toml")

	data, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("ReadFile(tool_config) error = %v; tool_config should remain in tmpDir", err)
	}

	if len(data) == 0 {
		t.Fatal("tool_config file is empty")
	}
}
