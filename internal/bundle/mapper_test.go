package bundle

import (
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestProviderMapper_Claude_MapAsset(t *testing.T) {
	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	mapper := NewProviderMapper(spec)
	workDir := "/project"

	tests := []struct {
		name    string
		layer   client.BundleLayer
		want    string
		wantErr bool
	}{
		{
			name:  "skill",
			layer: client.BundleLayer{AssetType: "skill", LogicalPath: "web-search/SKILL.md"},
			want:  filepath.Join(workDir, ".claude", "skills", "web-search", "SKILL.md"),
		},
		{
			name:  "agent_definition",
			layer: client.BundleLayer{AssetType: "agent_definition", LogicalPath: "researcher.md"},
			want:  filepath.Join(workDir, ".claude", "agents", "researcher.md"),
		},
		{
			name:  "tool_config",
			layer: client.BundleLayer{AssetType: "tool_config", LogicalPath: "mcp.json"},
			want:  filepath.Join(workDir, ".mcp.json"),
		},
		{
			name:    "unknown type",
			layer:   client.BundleLayer{AssetType: "unknown", LogicalPath: "test.txt"},
			wantErr: true,
		},
		{
			name:    "path traversal",
			layer:   client.BundleLayer{AssetType: "skill", LogicalPath: "../../../etc/passwd"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mapper.MapAsset(workDir, tc.layer)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MapAsset() expected error, got %q", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("MapAsset() unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("MapAsset() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderMapper_Codex_MapAsset(t *testing.T) {
	spec, ok := harness.GetProvider("codex")
	if !ok {
		t.Fatal("codex provider not found")
	}

	mapper := NewProviderMapper(spec)
	workDir := "/project"

	tests := []struct {
		name    string
		layer   client.BundleLayer
		want    string
		wantErr bool
	}{
		{
			name:  "skill",
			layer: client.BundleLayer{AssetType: "skill", LogicalPath: "web-search/SKILL.md"},
			want:  filepath.Join(workDir, ".agents", "skills", "web-search", "SKILL.md"),
		},
		{
			name:  "agent_definition",
			layer: client.BundleLayer{AssetType: "agent_definition", LogicalPath: "researcher.md"},
			want:  filepath.Join(workDir, ".codex", "agents", "researcher.md"),
		},
		{
			name:  "tool_config",
			layer: client.BundleLayer{AssetType: "tool_config", LogicalPath: "config.toml"},
			want:  filepath.Join(workDir, ".codex", "config.toml"),
		},
		{
			name:    "unknown type",
			layer:   client.BundleLayer{AssetType: "unknown", LogicalPath: "test.txt"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mapper.MapAsset(workDir, tc.layer)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MapAsset() expected error, got %q", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("MapAsset() unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("MapAsset() = %q, want %q", got, tc.want)
			}
		})
	}
}
