//go:build unix

package harness

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func TestBuildMCPProviderSpecs(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(10 * time.Minute)
	expired := now.Add(10 * time.Second)

	cfg := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_linear",
					TokenType:   "bearer",
					ExpiresAt:   &exp,
				},
			},
			"github": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: false},
				MCP:    &client.RunnerProviderMCP{URL: "https://example.com/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_gh",
				},
			},
			"jira": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://jira.example/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_jira",
					ExpiresAt:   &expired,
				},
			},
		},
	}

	specs := buildMCPProviderSpecs(cfg, now)
	if len(specs) != 1 {
		t.Fatalf("spec count = %d, want 1", len(specs))
	}

	if specs[0].Name != "linear" {
		t.Fatalf("provider = %q, want linear", specs[0].Name)
	}
}

func TestCreateClaudeMCPConfigFile(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(10 * time.Minute)
	cfg := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_linear",
					TokenType:   "bearer",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	path, sig, cleanup, err := createClaudeMCPConfigFile(cfg, now)
	if err != nil {
		t.Fatalf("createClaudeMCPConfigFile() error = %v", err)
	}

	if path == "" {
		t.Fatalf("expected non-empty path")
	}

	if sig == "" {
		t.Fatalf("expected non-empty signature")
	}

	if cleanup == nil {
		t.Fatalf("expected cleanup callback")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mcp file: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal mcp file: %v", err)
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("missing mcpServers")
	}

	if _, ok := mcpServers["linear"]; !ok {
		t.Fatalf("missing linear server entry")
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config file removed, stat err: %v", err)
	}
}

func TestNormalizeRefreshInterval(t *testing.T) {
	if got := normalizeRefreshInterval(0); got != 300*time.Second {
		t.Fatalf("normalize(0) = %s, want 300s", got)
	}

	if got := normalizeRefreshInterval(10); got != 60*time.Second {
		t.Fatalf("normalize(10) = %s, want 60s", got)
	}

	if got := normalizeRefreshInterval(3600); got != 900*time.Second {
		t.Fatalf("normalize(3600) = %s, want 900s", got)
	}
}

func TestLoadedMCPProviderNames(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(10 * time.Minute)

	cfg := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"zeta": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://zeta.example/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_zeta",
					ExpiresAt:   &exp,
				},
			},
			"alpha": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://alpha.example/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_alpha",
					ExpiresAt:   &exp,
				},
			},
			"disabled": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: false},
				MCP:    &client.RunnerProviderMCP{URL: "https://disabled.example/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok_disabled",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	names := loadedMCPProviderNames(cfg, now)
	if len(names) != 2 {
		t.Fatalf("name count = %d, want 2", len(names))
	}

	if names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("names = %#v, want [alpha zeta]", names)
	}
}

func TestLoadedMCPServers_ExportedWrapper(t *testing.T) {
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	exp := now.Add(5 * time.Minute)
	cfg := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	names := LoadedMCPServers(cfg, now)
	if len(names) != 1 || names[0] != "linear" {
		t.Fatalf("LoadedMCPServers = %#v, want [linear]", names)
	}
}
