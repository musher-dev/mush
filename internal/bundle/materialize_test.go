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

// storeTestAsset stores data as a blob and returns the SHA256 digest.
func storeTestAsset(t *testing.T, data string) string {
	t.Helper()

	digest, err := StoreBlob([]byte(data))
	if err != nil {
		t.Fatalf("StoreBlob() error = %v", err)
	}

	return digest
}

func TestInstallFromCache_CodexIndividualAgentsAndToolConfig(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	workDir := t.TempDir()

	skillDigest := storeTestAsset(t, "skill")
	agentADigest := storeTestAsset(t, "Agent A")
	agentBDigest := storeTestAsset(t, "Agent B")
	toolADigest := storeTestAsset(t, "[mcp_servers.alpha]\ncommand = \"a\"\n")
	toolBDigest := storeTestAsset(t, "[mcp_servers.beta]\ncommand = \"b\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
			{LogicalPath: "agents/researcher.md", AssetType: "agent_definition", ContentSHA256: agentADigest},
			{LogicalPath: "agents/reviewer.md", AssetType: "agent_definition", ContentSHA256: agentBDigest},
			{LogicalPath: "tools/a.toml", AssetType: "tool_config", ContentSHA256: toolADigest},
			{LogicalPath: "tools/b.toml", AssetType: "tool_config", ContentSHA256: toolBDigest},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	installed, installErr := InstallFromCache(workDir, manifest, NewProviderMapper(codexSpec), false)
	if installErr != nil {
		t.Fatalf("InstallFromCache() error = %v", installErr)
	}

	if len(installed) != 4 {
		t.Fatalf("InstallFromCache() installed %d paths, want 4", len(installed))
	}

	// Verify individual agent files (agents/ prefix is stripped by stripMatchingPrefix).
	for _, name := range []string{"researcher.md", "reviewer.md"} {
		agentPath := filepath.Join(workDir, ".codex", "agents", name)

		if _, statErr := os.Stat(agentPath); statErr != nil {
			t.Fatalf("agent file %s not found: %v", name, statErr)
		}
	}

	configData, readErr := os.ReadFile(filepath.Join(workDir, ".codex", "config.toml"))
	if readErr != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", readErr)
	}

	config := string(configData)
	if !strings.Contains(config, "[mcp_servers.alpha]") || !strings.Contains(config, "[mcp_servers.beta]") {
		t.Fatalf("config.toml missing merged servers: %s", config)
	}
}

func TestInstallFromCache_Conflict(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	workDir := t.TempDir()
	skillDigest := storeTestAsset(t, "new")

	existingPath := filepath.Join(workDir, ".claude", "skills", "web", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(existing) error = %v", err)
	}

	if err := os.WriteFile(existingPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	_, installErr := InstallFromCache(workDir, manifest, NewProviderMapper(claudeSpec), false)
	if installErr == nil {
		t.Fatal("InstallFromCache() expected conflict error, got nil")
	}

	var conflict *InstallConflictError
	if !errors.As(installErr, &conflict) {
		t.Fatalf("InstallFromCache() error type = %T, want *InstallConflictError", installErr)
	}
}

func TestInjectAssetsForLoad_HappyPath(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	agentDigest := storeTestAsset(t, "Agent content")
	skillDigest := storeTestAsset(t, "Skill content")
	toolDigest := storeTestAsset(t, `{"mcpServers":{}}`)

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "architect.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
			{LogicalPath: "skills/web/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
			{LogicalPath: "tools/mcp.json", AssetType: "tool_config", ContentSHA256: toolDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Both agent and skill should be injected; tool_config should NOT.
	if len(injected) != 2 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 2; got %v", len(injected), injected)
	}

	agentPath := filepath.Join(projectDir, ".claude", "agents", "architect.md")

	data, readErr := os.ReadFile(agentPath)
	if readErr != nil {
		t.Fatalf("agent file not found: %v", readErr)
	}

	if string(data) != "Agent content" {
		t.Fatalf("agent content = %q, want %q", string(data), "Agent content")
	}

	skillPath := filepath.Join(projectDir, ".claude", "skills", "web", "SKILL.md")

	data, readErr = os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("skill file not found: %v", readErr)
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
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()
	agentDigest := storeTestAsset(t, "New agent")

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
			{LogicalPath: "architect.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	// Should be skipped since the file already exists.
	if len(injected) != 0 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 0", len(injected))
	}

	// Original content should be preserved.
	data, readErr := os.ReadFile(existingPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
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
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	agentADigest := storeTestAsset(t, "Agent A")
	agentBDigest := storeTestAsset(t, "Agent B")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "architect.md", AssetType: "agent_definition", ContentSHA256: agentADigest},
			{LogicalPath: "reviewer.md", AssetType: "agent_definition", ContentSHA256: agentBDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
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
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()
	agentDigest := storeTestAsset(t, "Nested agent")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "agents/shaping-architect.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, _, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 1 {
		t.Fatalf("InjectAssetsForLoad() injected %d, want 1", len(injected))
	}

	// Should be at .claude/agents/shaping-architect.md (agents/ prefix stripped by stripMatchingPrefix).
	targetPath := filepath.Join(projectDir, ".claude", "agents", "shaping-architect.md")

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("nested agent file not found: %v", readErr)
	}

	if string(data) != "Nested agent" {
		t.Fatalf("nested agent content = %q, want %q", string(data), "Nested agent")
	}
}

func TestInjectAssetsForLoad_SkillFrontmatterWarning(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	// Skill with invalid YAML frontmatter (unquoted colon in value) — repairable.
	badDigest := storeTestAsset(t, "---\nname: test\ndescription: something: broken\n---\n# Skill\n")

	// Skill with valid YAML frontmatter.
	goodDigest := storeTestAsset(t, "---\nname: good\ndescription: \"works fine\"\n---\n# Skill\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/bad/SKILL.md", AssetType: "skill", ContentSHA256: badDigest},
			{LogicalPath: "skills/good/SKILL.md", AssetType: "skill", ContentSHA256: goodDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, warnings, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 2 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 2; got %v", len(injected), injected)
	}

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
	skillPath := filepath.Join(projectDir, ".claude", "skills", "bad", "SKILL.md")

	data, readErr := os.ReadFile(skillPath)
	if readErr != nil {
		t.Fatalf("ReadFile(repaired skill) error = %v", readErr)
	}

	if !strings.Contains(string(data), `description: "something: broken"`) {
		t.Fatalf("injected skill should have repaired frontmatter, got: %s", string(data))
	}
}

func TestInjectAssetsForLoad_SkillFrontmatterUnrepairable(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	// Skill with unrepairable YAML frontmatter (bad indentation).
	brokenDigest := storeTestAsset(t, "---\nname: test\n  bad:\n indent\n---\n# Skill\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "skills/broken/SKILL.md", AssetType: "skill", ContentSHA256: brokenDigest},
		},
	}

	claudeSpec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(claudeSpec)

	injected, warnings, cleanup, err := InjectAssetsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectAssetsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 1 {
		t.Fatalf("InjectAssetsForLoad() injected %d paths, want 1; got %v", len(injected), injected)
	}

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
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	toolADigest := storeTestAsset(t, "[mcp_servers.alpha]\ncommand = \"a\"\n")
	toolBDigest := storeTestAsset(t, "[mcp_servers.beta]\ncommand = \"b\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "tools/a.toml", AssetType: "tool_config", ContentSHA256: toolADigest},
			{LogicalPath: "tools/b.toml", AssetType: "tool_config", ContentSHA256: toolBDigest},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 1 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d paths, want 1; got %v", len(injected), injected)
	}

	// Verify merged config file.
	configPath := filepath.Join(projectDir, ".codex", "config.toml")

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", readErr)
	}

	config := string(data)
	if !strings.Contains(config, "alpha") || !strings.Contains(config, "beta") {
		t.Fatalf("config.toml missing merged servers: %s", config)
	}
}

func TestInjectToolConfigsForLoad_BackupRestore(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	// Pre-create an existing config file.
	existingPath := filepath.Join(projectDir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	originalContent := "[mcp_servers.existing]\ncommand = \"original\"\n"

	if err := os.WriteFile(existingPath, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	toolDigest := storeTestAsset(t, "[mcp_servers.new_server]\ncommand = \"new\"\n")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "tools/new.toml", AssetType: "tool_config", ContentSHA256: toolDigest},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	if len(injected) != 1 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d, want 1", len(injected))
	}

	// Verify merged content before cleanup.
	data, readErr := os.ReadFile(existingPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	merged := string(data)
	if !strings.Contains(merged, "existing") || !strings.Contains(merged, "new_server") {
		t.Fatalf("merged config missing servers: %s", merged)
	}

	// Run cleanup — should restore original content.
	cleanup()

	data, readErr = os.ReadFile(existingPath)
	if readErr != nil {
		t.Fatalf("ReadFile() after cleanup error = %v", readErr)
	}

	if string(data) != originalContent {
		t.Fatalf("config after cleanup = %q, want %q", string(data), originalContent)
	}
}

func TestInjectToolConfigsForLoad_NoToolConfigs(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()
	agentDigest := storeTestAsset(t, "Agent content")

	manifest := &client.BundleManifest{
		Layers: []client.BundleLayer{
			{LogicalPath: "researcher.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
		},
	}

	codexSpec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(codexSpec)

	injected, cleanup, err := InjectToolConfigsForLoad(projectDir, manifest, mapper)
	if err != nil {
		t.Fatalf("InjectToolConfigsForLoad() error = %v", err)
	}

	defer cleanup()

	if len(injected) != 0 {
		t.Fatalf("InjectToolConfigsForLoad() injected %d paths, want 0", len(injected))
	}
}
