package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestInstallFromCache_CodexMergesAgentsAndToolConfig(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := t.TempDir()

	assetsDir := filepath.Join(cacheDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	write := func(rel, data string) {
		path := filepath.Join(assetsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}

		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}

	write("skills/web/SKILL.md", "skill")
	write("agents/researcher.md", "Agent A")
	write("agents/reviewer.md", "Agent B")
	write("tools/a.toml", "[mcp_servers.alpha]\ncommand = \"a\"\n")
	write("tools/b.toml", "[mcp_servers.beta]\ncommand = \"b\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
			{LogicalPath: "agents/researcher.md", AssetType: "agent_definition"},
			{LogicalPath: "agents/reviewer.md", AssetType: "agent_definition"},
			{LogicalPath: "tools/a.toml", AssetType: "tool_config"},
			{LogicalPath: "tools/b.toml", AssetType: "tool_config"},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	installed, err := InstallFromCache(workDir, cacheDir, manifest, NewProviderMapper(codexSpec), false)
	if err != nil {
		t.Fatalf("InstallFromCache() error = %v", err)
	}

	if len(installed) != 3 {
		t.Fatalf("InstallFromCache() installed %d paths, want 3", len(installed))
	}

	agentsData, err := os.ReadFile(filepath.Join(workDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}

	agents := string(agentsData)
	if !strings.Contains(agents, "Bundle Agent: agents/researcher.md") || !strings.Contains(agents, "Bundle Agent: agents/reviewer.md") {
		t.Fatalf("AGENTS.md missing merged agent markers: %s", agents)
	}

	configData, err := os.ReadFile(filepath.Join(workDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}

	config := string(configData)
	if !strings.Contains(config, "[mcp_servers.alpha]") || !strings.Contains(config, "[mcp_servers.beta]") {
		t.Fatalf("config.toml missing merged servers: %s", config)
	}
}

func TestInstallFromCache_Conflict(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := t.TempDir()

	assetsDir := filepath.Join(cacheDir, "assets", "skills", "web")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(assetsDir, "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(cache) error = %v", err)
	}

	existingPath := filepath.Join(workDir, ".claude", "skills", "skills", "web", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(existing) error = %v", err)
	}

	if err := os.WriteFile(existingPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	_, err := InstallFromCache(workDir, cacheDir, manifest, NewProviderMapper(claudeSpec), false)
	if err == nil {
		t.Fatal("InstallFromCache() expected conflict error, got nil")
	}

	var conflict *InstallConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("InstallFromCache() error type = %T, want *InstallConflictError", err)
	}
}
