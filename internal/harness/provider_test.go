package harness

import (
	"testing"
)

func TestProviderSpecsLoaded(t *testing.T) {
	names := ProviderNames()
	if len(names) < 3 {
		t.Fatalf("expected at least 3 providers, got %d: %v", len(names), names)
	}

	expected := []string{"bash", "claude", "codex"}
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
}

func TestGetProvider_Bash(t *testing.T) {
	spec, ok := GetProvider("bash")
	if !ok {
		t.Fatal("bash provider not found")
	}

	if spec.Binary != "" {
		t.Fatalf("Binary = %q, want empty (always available)", spec.Binary)
	}

	if spec.Assets != nil {
		t.Fatal("expected Assets to be nil for bash")
	}

	if spec.MCP != nil {
		t.Fatal("expected MCP to be nil for bash")
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

	if HasAssetMapping("bash") {
		t.Fatal("expected bash to NOT have asset mapping")
	}

	if HasAssetMapping("nonexistent") {
		t.Fatal("expected nonexistent to NOT have asset mapping")
	}
}

func TestAvailableFunc_BashAlwaysAvailable(t *testing.T) {
	fn := AvailableFunc("bash")
	if !fn() {
		t.Fatal("bash should always be available (no binary required)")
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
	fn := AvailableFunc("bash")

	// Call multiple times to ensure consistency.
	for range 3 {
		if !fn() {
			t.Fatal("bash should always be available")
		}
	}
}
