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

func TestInstallFromCache_CodexIndividualAgentsAndToolConfig(t *testing.T) {
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

	if len(installed) != 4 {
		t.Fatalf("InstallFromCache() installed %d paths, want 4", len(installed))
	}

	// Verify individual agent files.
	for _, name := range []string{"agents/researcher.md", "agents/reviewer.md"} {
		agentPath := filepath.Join(workDir, ".codex", "agents", name)

		if _, statErr := os.Stat(agentPath); statErr != nil {
			t.Fatalf("agent file %s not found: %v", name, statErr)
		}
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

func TestInjectAssetsForLoad_HappyPath(t *testing.T) {
	projectDir := t.TempDir()
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

	write("architect.md", "Agent content")
	write("skills/web/SKILL.md", "Skill content")
	write("tools/mcp.json", `{"mcpServers":{}}`)

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "architect.md", AssetType: "agent_definition"},
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill"},
			{LogicalPath: "tools/mcp.json", AssetType: "tool_config"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Both agent and skill should be injected; tool_config should NOT.
	if len(injected) != 2 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 2; got %v", len(injected), injected)
	}

	agentPath := filepath.Join(projectDir, ".claude", "agents", "architect.md")

	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("agent file not found: %v", err)
	}

	if string(data) != "Agent content" {
		t.Fatalf("agent content = %q, want %q", string(data), "Agent content")
	}

	skillPath := filepath.Join(projectDir, ".claude", "skills", "skills", "web", "SKILL.md")

	data, err = os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("skill file not found: %v", err)
	}

	if string(data) != "Skill content" {
		t.Fatalf("skill content = %q, want %q", string(data), "Skill content")
	}

	// tool_config should NOT be injected into the project dir.
	toolPath := filepath.Join(projectDir, ".mcp.json")
	if _, statErr := os.Stat(toolPath); statErr == nil {
		t.Fatal("tool_config should not be injected into project dir")
	}
}

func TestInjectAssetsForLoad_SkipsExisting(t *testing.T) {
	projectDir := t.TempDir()
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

	write("architect.md", "New agent")

	// Pre-create the agent file in the project directory.
	existingPath := filepath.Join(projectDir, ".claude", "agents", "architect.md")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(existingPath, []byte("User agent"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "architect.md", AssetType: "agent_definition"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Should be skipped since the file already exists.
	if len(injected) != 0 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 0", len(injected))
	}

	// Original content should be preserved.
	data, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != "User agent" {
		t.Fatalf("existing agent content = %q, want %q", string(data), "User agent")
	}

	// Cleanup should NOT remove the pre-existing file.
	cleanup()

	if _, statErr := os.Stat(existingPath); statErr != nil {
		t.Fatal("cleanup removed pre-existing agent file")
	}
}

func TestInjectAssetsForLoad_Cleanup(t *testing.T) {
	projectDir := t.TempDir()
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

	write("architect.md", "Agent A")
	write("reviewer.md", "Agent B")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "architect.md", AssetType: "agent_definition"},
			{LogicalPath: "reviewer.md", AssetType: "agent_definition"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	if len(injected) != 2 {
		t.Fatalf("InjectAssetsForLoad() injected %d, want 2", len(injected))
	}

	// Verify files exist before cleanup.
	for _, rel := range injected {
		path := filepath.Join(projectDir, rel)
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("injected file %s not found before cleanup", rel)
		}
	}

	// Run cleanup.
	cleanup()

	// Verify files removed after cleanup.
	for _, rel := range injected {
		path := filepath.Join(projectDir, rel)
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatalf("injected file %s still exists after cleanup", rel)
		}
	}

	// .claude/agents/ directory should also be cleaned up since we created it.
	agentsDir := filepath.Join(projectDir, ".claude", "agents")
	if _, statErr := os.Stat(agentsDir); statErr == nil {
		t.Fatal(".claude/agents/ directory still exists after cleanup")
	}
}

func TestInjectAssetsForLoad_NestedLogicalPath(t *testing.T) {
	projectDir := t.TempDir()
	cacheDir := t.TempDir()

	assetsDir := filepath.Join(cacheDir, "assets")

	// Create a nested path like "agents/shaping-architect.md".
	nestedPath := filepath.Join(assetsDir, "agents", "shaping-architect.md")
	if err := os.MkdirAll(filepath.Dir(nestedPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(nestedPath, []byte("Nested agent"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "agents/shaping-architect.md", AssetType: "agent_definition"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 1 {
		t.Fatalf("InjectAssetsForLoad() injected %d, want 1", len(injected))
	}

	// Should be at .claude/agents/agents/shaping-architect.md (mapper joins AgentDir + LogicalPath).
	targetPath := filepath.Join(projectDir, ".claude", "agents", "agents", "shaping-architect.md")

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("nested agent file not found: %v", err)
	}

	if string(data) != "Nested agent" {
		t.Fatalf("nested agent content = %q, want %q", string(data), "Nested agent")
	}
}

func TestInjectAssetsForLoad_SkillFrontmatterWarning(t *testing.T) {
	projectDir := t.TempDir()
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

	// Skill with invalid YAML frontmatter (unquoted colon in value) — repairable.
	write("skills/bad/SKILL.md", "---\nname: test\ndescription: something: broken\n---\n# Skill\n")

	// Skill with valid YAML frontmatter.
	write("skills/good/SKILL.md", "---\nname: good\ndescription: \"works fine\"\n---\n# Skill\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/bad/SKILL.md", AssetType: "skill"},
			{LogicalPath: "skills/good/SKILL.md", AssetType: "skill"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, warnings, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Both should still be injected (warnings are non-fatal).
	if len(injected) != 2 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 2; got %v", len(injected), injected)
	}

	// Should have exactly one warning for the auto-repaired skill.
	if len(warnings) != 1 {
		t.Fatalf("InjectAssetsForLoad() warnings = %d, want 1; got %v", len(warnings), warnings)
	}

	if !strings.Contains(warnings[0], "skills/bad/SKILL.md") {
		t.Fatalf("warning should mention bad skill path, got: %s", warnings[0])
	}

	if !strings.Contains(warnings[0], "auto-repaired") {
		t.Fatalf("warning should mention auto-repaired, got: %s", warnings[0])
	}

	// Verify the injected file has repaired content.
	skillPath := filepath.Join(projectDir, ".claude", "skills", "skills", "bad", "SKILL.md")

	data, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("ReadFile(repaired skill) error = %v", readErr)
	}

	if !strings.Contains(string(data), `description: "something: broken"`) {
		t.Fatalf("injected skill should have repaired frontmatter, got: %s", string(data))
	}
}

func TestInjectAssetsForLoad_SkillFrontmatterUnrepairable(t *testing.T) {
	projectDir := t.TempDir()
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

	// Skill with unrepairable YAML frontmatter (bad indentation).
	write("skills/broken/SKILL.md", "---\nname: test\n  bad:\n indent\n---\n# Skill\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/broken/SKILL.md", AssetType: "skill"},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, warnings, cleanup, err := InjectAssetsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Should still be injected (warnings are non-fatal).
	if len(injected) != 1 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 1; got %v", len(injected), injected)
	}

	// Should have one warning mentioning strict harnesses.
	if len(warnings) != 1 {
		t.Fatalf("InjectAssetsForLoad() warnings = %d, want 1; got %v", len(warnings), warnings)
	}

	if !strings.Contains(warnings[0], "strict harnesses") {
		t.Fatalf("warning should mention strict harnesses, got: %s", warnings[0])
	}

	if !strings.Contains(warnings[0], "skills/broken/SKILL.md") {
		t.Fatalf("warning should mention broken skill path, got: %s", warnings[0])
	}
}

func TestInjectToolConfigsForLoad_HappyPath(t *testing.T) {
	projectDir := t.TempDir()
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

	write("tools/a.toml", "[mcp_servers.alpha]\ncommand = \"a\"\n")
	write("tools/b.toml", "[mcp_servers.beta]\ncommand = \"b\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "tools/a.toml", AssetType: "tool_config"},
			{LogicalPath: "tools/b.toml", AssetType: "tool_config"},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 1 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d paths, want 1; got %v", len(injected), injected)
	}

	// Verify merged config file.
	configPath := filepath.Join(projectDir, ".codex", "config.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}

	config := string(data)
	if !strings.Contains(config, "alpha") || !strings.Contains(config, "beta") {
		t.Fatalf("config.toml missing merged servers: %s", config)
	}
}

func TestInjectToolConfigsForLoad_BackupRestore(t *testing.T) {
	projectDir := t.TempDir()
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

	// Pre-create an existing config file.
	existingPath := filepath.Join(projectDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	originalContent := "[mcp_servers.existing]\ncommand = \"original\"\n"

	if err := os.WriteFile(existingPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	write("tools/new.toml", "[mcp_servers.new_server]\ncommand = \"new\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "tools/new.toml", AssetType: "tool_config"},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	if len(injected) != 1 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d, want 1", len(injected))
	}

	// Verify merged content before cleanup.
	data, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	merged := string(data)
	if !strings.Contains(merged, "existing") || !strings.Contains(merged, "new_server") {
		t.Fatalf("merged config missing servers: %s", merged)
	}

	// Run cleanup — should restore original content.
	cleanup()

	data, err = os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("ReadFile() after cleanup error = %v", err)
	}

	if string(data) != originalContent {
		t.Fatalf("config after cleanup = %q, want %q", string(data), originalContent)
	}
}

func TestInjectToolConfigsForLoad_NoToolConfigs(t *testing.T) {
	projectDir := t.TempDir()
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

	write("researcher.md", "Agent content")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "researcher.md", AssetType: "agent_definition"},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, cacheDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 0 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d paths, want 0", len(injected))
	}
}
