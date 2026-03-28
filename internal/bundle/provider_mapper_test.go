package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestProviderMapper_PrepareLoad_Claude(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(spec)

	skillDigest := storeTestAsset(t, "skill content")
	agentDigest := storeTestAsset(t, "agent content")
	toolDigest := storeTestAsset(t, `{"mcpServers":{}}`)

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
			{LogicalPath: "researcher.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
			{LogicalPath: "mcp.json", AssetType: "tool_config", ContentSHA256: toolDigest},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), manifest)
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

	data, readErr := os.ReadFile(toolPath)
	if readErr != nil {
		t.Fatalf("ReadFile(tool_config) error = %v; tool_config should remain in tmpDir", readErr)
	}

	if len(data) == 0 {
		t.Fatal("tool_config file is empty")
	}
}

func TestProviderMapper_PrepareLoad_Claude_WithOtherAsset(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(spec)

	skillDigest := storeTestAsset(t, "skill content")
	refDigest := storeTestAsset(t, "reference content")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/writing/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
			{LogicalPath: "skills/writing/references/format.md", AssetType: "other", ContentSHA256: refDigest},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), manifest)
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

	data, readErr := os.ReadFile(otherPath)
	if readErr != nil {
		t.Fatalf("ReadFile(other) error = %v", readErr)
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
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	spec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(spec)

	agentADigest := storeTestAsset(t, "Agent A")
	agentBDigest := storeTestAsset(t, "Agent B")
	toolDigest := storeTestAsset(t, "[mcp_servers.test]\ncommand = \"test\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "researcher.md", AssetType: "agent_definition", ContentSHA256: agentADigest},
			{LogicalPath: "reviewer.md", AssetType: "agent_definition", ContentSHA256: agentBDigest},
			{LogicalPath: "tools/mcp.toml", AssetType: "tool_config", ContentSHA256: toolDigest},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), manifest)
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

	data, readErr := os.ReadFile(toolPath)
	if readErr != nil {
		t.Fatalf("ReadFile(tool_config) error = %v; tool_config should remain in tmpDir", readErr)
	}

	if len(data) == 0 {
		t.Fatal("tool_config file is empty")
	}
}
