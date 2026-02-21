package bundle

import (
	"os"
	"path/filepath"
	"strings"
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

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
			{LogicalPath: "researcher.md", AssetType: "agent_definition"},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), cacheDir, manifest)
	if err != nil {
		t.Fatalf("PrepareLoad error = %v", err)
	}

	defer cleanup()

	// Claude agents are individual files (not merged).
	agentPath := filepath.Join(tmpDir, ".claude", "agents", "researcher.md")

	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("ReadFile(agent) error = %v", err)
	}

	if string(data) != "agent content" {
		t.Fatalf("agent content = %q, want %q", string(data), "agent content")
	}

	// Skill: logical path "skills/web/SKILL.md" maps to .claude/skills/skills/web/SKILL.md
	skillPath := filepath.Join(tmpDir, ".claude", "skills", "skills", "web", "SKILL.md")

	data, err = os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(skill) error = %v", err)
	}

	if string(data) != "skill content" {
		t.Fatalf("skill content = %q, want %q", string(data), "skill content")
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

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "researcher.md", AssetType: "agent_definition"},
			{LogicalPath: "reviewer.md", AssetType: "agent_definition"},
		},
	}

	tmpDir, cleanup, err := mapper.PrepareLoad(t.Context(), cacheDir, manifest)
	if err != nil {
		t.Fatalf("PrepareLoad error = %v", err)
	}

	defer cleanup()

	// Codex agents are merged into AGENTS.md.
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Bundle Agent: researcher.md") {
		t.Fatalf("AGENTS.md missing researcher agent: %s", content)
	}

	if !strings.Contains(content, "Bundle Agent: reviewer.md") {
		t.Fatalf("AGENTS.md missing reviewer agent: %s", content)
	}
}
