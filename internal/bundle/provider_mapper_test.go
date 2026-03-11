package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

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

	agentPath := filepath.Join(tmpDir, ".claude", "agents", "researcher.md")
	if _, statErr := os.Stat(agentPath); statErr != nil {
		t.Fatalf("agent_definition should be materialized in tmpDir for add_dir mode harness: %v", statErr)
	}

	skillPath := filepath.Join(tmpDir, ".claude", "skills", "web", "SKILL.md")
	if _, statErr := os.Stat(skillPath); statErr != nil {
		t.Fatalf("skill should be materialized in tmpDir for add_dir mode harness: %v", statErr)
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

func TestProviderMapper_PrepareLoad_Claude_WithOtherAsset(t *testing.T) {
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

	write("skills/writing/SKILL.md", "skill content")
	write("skills/writing/references/format.md", "reference content")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/writing/SKILL.md", AssetType: "skill"},
			{LogicalPath: "skills/writing/references/format.md", AssetType: "other"},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), cacheDir, manifest)
	if err != nil {
		t.Fatalf("PrepareLoad error = %v", err)
	}

	defer cleanup()

	skillPath := filepath.Join(tmpDir, ".claude", "skills", "writing", "SKILL.md")
	if _, statErr := os.Stat(skillPath); statErr != nil {
		t.Fatalf("skill should be materialized: %v", statErr)
	}

	otherPath := filepath.Join(tmpDir, "skills", "writing", "references", "format.md")
	if _, statErr := os.Stat(otherPath); statErr != nil {
		t.Fatalf("other asset should be materialized at logical path: %v", statErr)
	}

	data, err := os.ReadFile(otherPath)
	if err != nil {
		t.Fatalf("ReadFile(other) error = %v", err)
	}

	if string(data) != "reference content" {
		t.Fatalf("other asset content = %q, want %q", string(data), "reference content")
	}
}

func TestStripMatchingPrefix(t *testing.T) {
	tests := []struct {
		dir, logicalPath, want string
	}{
		{".claude/skills", "skills/web/SKILL.md", "web/SKILL.md"},
		{".claude/skills", "generate-skill/SKILL.md", "generate-skill/SKILL.md"},
		{".claude/skills", ".claude/skills/test-skill.md", "test-skill.md"},
		{".claude/skills", ".claude/skills/nested/deep.md", "nested/deep.md"},
		{".claude/agents", "agents/test-agent.md", "test-agent.md"},
		{".claude/agents", ".claude/agents/researcher.md", "researcher.md"},
		{".claude/agents", "researcher.md", "researcher.md"},
		{".codex/agents", "agents/a.md", "a.md"},
		{".codex/agents", "other/a.md", "other/a.md"},
	}

	for _, tt := range tests {
		got := stripMatchingPrefix(tt.dir, tt.logicalPath)
		if got != tt.want {
			t.Errorf("stripMatchingPrefix(%q, %q) = %q, want %q", tt.dir, tt.logicalPath, got, tt.want)
		}
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

	for _, name := range []string{"researcher.md", "reviewer.md"} {
		agentPath := filepath.Join(tmpDir, ".codex", "agents", name)
		if _, statErr := os.Stat(agentPath); statErr != nil {
			t.Fatalf("agent_definition %s should be materialized in tmpDir for add_dir mode harness: %v", name, statErr)
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
