package harness

import (
	"testing"
)

func TestProviderSpecsLoaded(t *testing.T) {
	names := ProviderNames()
	if len(names) < 6 {
		t.Fatalf("expected at least 6 providers, got %d: %v", len(names), names)
	}

	expected := []string{"claude", "codex", "copilot", "cursor", "gemini", "opencode"}
	for _, name := range expected {
		if _, ok := GetProvider(name); !ok {
			t.Fatalf("expected provider %q to be loaded", name)
		}
	}
}

func TestProviderSpecsAreSorted(t *testing.T) {
	names := ProviderNames()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("ProviderNames() not sorted: %v", names)
		}
	}
}

func TestGetProvider_Claude(t *testing.T) {
	spec, ok := GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	if spec.Name != "claude" {
		t.Fatalf("Name = %q, want claude", spec.Name)
	}

	if spec.Binary != "claude" {
		t.Fatalf("Binary = %q, want claude", spec.Binary)
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.SkillDir != ".claude/skills" {
		t.Fatalf("SkillDir = %q, want .claude/skills", spec.Assets.SkillDir)
	}

	if spec.Assets.AgentDir != ".claude/agents" {
		t.Fatalf("AgentDir = %q, want .claude/agents", spec.Assets.AgentDir)
	}

	if spec.Assets.ToolConfigFile != ".mcp.json" {
		t.Fatalf("ToolConfigFile = %q, want .mcp.json", spec.Assets.ToolConfigFile)
	}

	if spec.MCP == nil {
		t.Fatal("expected MCP to be non-nil")
	}

	if spec.MCP.Format != "json" {
		t.Fatalf("MCP.Format = %q, want json", spec.MCP.Format)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "add_dir" {
		t.Fatalf("BundleDir.Mode = %v, want add_dir", spec.BundleDir)
	}

	if spec.CLI == nil || spec.CLI.MCPConfig != "--mcp-config" {
		t.Fatalf("CLI.MCPConfig = %v, want --mcp-config", spec.CLI)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g @anthropic-ai/claude-code" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g @anthropic-ai/claude-code", spec.Status.InstallHint)
	}

	if len(spec.Status.VersionArgs) != 1 || spec.Status.VersionArgs[0] != "--version" {
		t.Fatalf("Status.VersionArgs = %v, want [--version]", spec.Status.VersionArgs)
	}

	if spec.Status.ConfigDir != "~/.claude" {
		t.Fatalf("Status.ConfigDir = %q, want ~/.claude", spec.Status.ConfigDir)
	}

	if spec.Status.AuthCheck == nil {
		t.Fatal("expected Status.AuthCheck to be non-nil")
	}

	if spec.Status.AuthCheck.Path != "~/.claude/.credentials.json" {
		t.Fatalf("Status.AuthCheck.Path = %q, want ~/.claude/.credentials.json", spec.Status.AuthCheck.Path)
	}
}

func TestGetProvider_Codex(t *testing.T) {
	spec, ok := GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.AgentDir != ".codex/agents" {
		t.Fatalf("AgentDir = %q, want .codex/agents", spec.Assets.AgentDir)
	}

	if spec.MCP == nil || spec.MCP.Format != "toml" {
		t.Fatalf("MCP.Format = %v, want toml", spec.MCP)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "add_dir" {
		t.Fatalf("BundleDir.Mode = %v, want add_dir", spec.BundleDir)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g @openai/codex" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g @openai/codex", spec.Status.InstallHint)
	}

	if spec.Status.AuthCheck != nil {
		t.Fatal("expected Status.AuthCheck to be nil for codex")
	}
}

func TestGetProvider_Copilot(t *testing.T) {
	spec, ok := GetProvider("copilot")
	if !ok {
		t.Fatal("copilot provider not found")
	}

	if spec.Binary != "copilot" {
		t.Fatalf("Binary = %q, want copilot", spec.Binary)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "add_dir" {
		t.Fatalf("BundleDir = %#v, want mode add_dir", spec.BundleDir)
	}

	if spec.BundleDir.Flag != "--add-dir" {
		t.Fatalf("BundleDir.Flag = %q, want --add-dir", spec.BundleDir.Flag)
	}

	if spec.CLI == nil || spec.CLI.MCPConfig != "--additional-mcp-config" {
		t.Fatalf("CLI = %#v, want mcpConfig --additional-mcp-config", spec.CLI)
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.SkillDir != ".github/skills" {
		t.Fatalf("SkillDir = %q, want .github/skills", spec.Assets.SkillDir)
	}

	if spec.Assets.AgentDir != ".github/agents" {
		t.Fatalf("AgentDir = %q, want .github/agents", spec.Assets.AgentDir)
	}

	if spec.Assets.ToolConfigFile != ".copilot/mcp-config.json" {
		t.Fatalf("ToolConfigFile = %q, want .copilot/mcp-config.json", spec.Assets.ToolConfigFile)
	}

	if spec.MCP == nil || spec.MCP.Format != "json" {
		t.Fatalf("MCP = %#v, want format json", spec.MCP)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g @github/copilot" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g @github/copilot", spec.Status.InstallHint)
	}

	if spec.Status.AuthCheck == nil {
		t.Fatal("expected Status.AuthCheck to be non-nil")
	}

	if spec.Status.AuthCheck.Path != "~/.copilot/oauth_token" {
		t.Fatalf("Status.AuthCheck.Path = %q, want ~/.copilot/oauth_token", spec.Status.AuthCheck.Path)
	}
}

func TestGetProvider_Cursor(t *testing.T) {
	spec, ok := GetProvider("cursor")
	if !ok {
		t.Fatal("cursor provider not found")
	}

	if spec.Binary != "cursor-agent" {
		t.Fatalf("Binary = %q, want cursor-agent", spec.Binary)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "cwd" {
		t.Fatalf("BundleDir = %#v, want mode cwd", spec.BundleDir)
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.SkillDir != ".cursor/rules" {
		t.Fatalf("SkillDir = %q, want .cursor/rules", spec.Assets.SkillDir)
	}

	if spec.Assets.AgentDir != ".cursor/agents" {
		t.Fatalf("AgentDir = %q, want .cursor/agents", spec.Assets.AgentDir)
	}

	if spec.Assets.ToolConfigFile != ".cursor/agent.json" {
		t.Fatalf("ToolConfigFile = %q, want .cursor/agent.json", spec.Assets.ToolConfigFile)
	}

	if spec.MCP == nil || spec.MCP.Format != "json" {
		t.Fatalf("MCP = %#v, want format json", spec.MCP)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g @cursor/agent" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g @cursor/agent", spec.Status.InstallHint)
	}

	if spec.Status.AuthCheck == nil {
		t.Fatal("expected Status.AuthCheck to be non-nil")
	}

	if spec.Status.AuthCheck.Path != "~/.cursor/credentials.json" {
		t.Fatalf("Status.AuthCheck.Path = %q, want ~/.cursor/credentials.json", spec.Status.AuthCheck.Path)
	}
}

func TestGetProvider_OpenCode(t *testing.T) {
	spec, ok := GetProvider("opencode")
	if !ok {
		t.Fatal("opencode provider not found")
	}

	if spec.Binary != "opencode" {
		t.Fatalf("Binary = %q, want opencode", spec.Binary)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "cwd" {
		t.Fatalf("BundleDir = %#v, want mode cwd", spec.BundleDir)
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.SkillDir != ".opencode/skills" {
		t.Fatalf("SkillDir = %q, want .opencode/skills", spec.Assets.SkillDir)
	}

	if spec.Assets.AgentDir != ".opencode/agents" {
		t.Fatalf("AgentDir = %q, want .opencode/agents", spec.Assets.AgentDir)
	}

	if spec.Assets.ToolConfigFile != "opencode.json" {
		t.Fatalf("ToolConfigFile = %q, want opencode.json", spec.Assets.ToolConfigFile)
	}

	if spec.MCP == nil || spec.MCP.Format != "json" {
		t.Fatalf("MCP = %#v, want format json", spec.MCP)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g opencode-ai" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g opencode-ai", spec.Status.InstallHint)
	}

	if spec.Status.AuthCheck == nil {
		t.Fatal("expected Status.AuthCheck to be non-nil")
	}

	if spec.Status.AuthCheck.Path != "~/.local/share/opencode/auth.json" {
		t.Fatalf("Status.AuthCheck.Path = %q, want ~/.local/share/opencode/auth.json", spec.Status.AuthCheck.Path)
	}
}

func TestGetProvider_Gemini(t *testing.T) {
	spec, ok := GetProvider("gemini")
	if !ok {
		t.Fatal("gemini provider not found")
	}

	if spec.Binary != "gemini" {
		t.Fatalf("Binary = %q, want gemini", spec.Binary)
	}

	if spec.BundleDir == nil || spec.BundleDir.Mode != "cwd" {
		t.Fatalf("BundleDir = %#v, want mode cwd", spec.BundleDir)
	}

	if spec.Assets == nil {
		t.Fatal("expected Assets to be non-nil")
	}

	if spec.Assets.SkillDir != ".gemini/commands" {
		t.Fatalf("SkillDir = %q, want .gemini/commands", spec.Assets.SkillDir)
	}

	if spec.Assets.AgentDir != ".gemini/commands" {
		t.Fatalf("AgentDir = %q, want .gemini/commands", spec.Assets.AgentDir)
	}

	if spec.Assets.ToolConfigFile != ".gemini/settings.json" {
		t.Fatalf("ToolConfigFile = %q, want .gemini/settings.json", spec.Assets.ToolConfigFile)
	}

	if spec.MCP == nil || spec.MCP.Format != "json" {
		t.Fatalf("MCP = %#v, want format json", spec.MCP)
	}

	if spec.Status == nil {
		t.Fatal("expected Status to be non-nil")
	}

	if spec.Status.InstallHint != "npm install -g @google/gemini-cli" {
		t.Fatalf("Status.InstallHint = %q, want npm install -g @google/gemini-cli", spec.Status.InstallHint)
	}

	if spec.Status.AuthCheck == nil {
		t.Fatal("expected Status.AuthCheck to be non-nil")
	}

	if spec.Status.AuthCheck.Path != "~/.gemini/oauth_creds.json" {
		t.Fatalf("Status.AuthCheck.Path = %q, want ~/.gemini/oauth_creds.json", spec.Status.AuthCheck.Path)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	_, ok := GetProvider("nonexistent")
	if ok {
		t.Fatal("expected nonexistent provider to not be found")
	}
}

func TestHasAssetMapping(t *testing.T) {
	if !HasAssetMapping("claude") {
		t.Fatal("expected claude to have asset mapping")
	}

	if !HasAssetMapping("codex") {
		t.Fatal("expected codex to have asset mapping")
	}

	if !HasAssetMapping("copilot") {
		t.Fatal("expected copilot to have asset mapping")
	}

	if !HasAssetMapping("cursor") {
		t.Fatal("expected cursor to have asset mapping")
	}

	if !HasAssetMapping("gemini") {
		t.Fatal("expected gemini to have asset mapping")
	}

	if !HasAssetMapping("opencode") {
		t.Fatal("expected opencode to have asset mapping")
	}

	if HasAssetMapping("nonexistent") {
		t.Fatal("expected nonexistent to NOT have asset mapping")
	}
}

func TestAvailableFunc_Nonexistent(t *testing.T) {
	fn := AvailableFunc("nonexistent")
	if fn() {
		t.Fatal("nonexistent provider should not be available")
	}
}

func TestAvailableFunc_LazyEvaluation(t *testing.T) {
	// Verify the closure captures name, not spec — it's lazy.
	fn := AvailableFunc("claude")

	// Call multiple times to ensure consistency.
	for range 3 {
		result := fn()
		_ = result // Just verify it doesn't panic; availability depends on PATH.
	}
}
