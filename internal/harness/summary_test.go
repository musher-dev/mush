//go:build unix

package harness

import (
	"testing"

	"github.com/musher-dev/mush/internal/client"
)

func TestSummarizeBundleManifest_SkillNames(t *testing.T) {
	tests := []struct {
		name        string
		logicalPath string
		assetType   string
		wantName    string
	}{
		{
			name:        "nested skill with descriptive parent",
			logicalPath: "skills/web-search/SKILL.md",
			assetType:   "skill",
			wantName:    "web-search",
		},
		{
			name:        "deeply nested with .claude prefix",
			logicalPath: ".claude/skills/context-brief/SKILL.md",
			assetType:   "skill",
			wantName:    "context-brief",
		},
		{
			name:        "bare SKILL.md with no parent",
			logicalPath: "SKILL.md",
			assetType:   "skill",
			wantName:    "SKILL.md",
		},
		{
			name:        "skill directly under generic skills dir",
			logicalPath: "skills/SKILL.md",
			assetType:   "skill",
			wantName:    "SKILL.md",
		},
		{
			name:        "non-conventional skill filename",
			logicalPath: "review-code.md",
			assetType:   "skill",
			wantName:    "review-code.md",
		},
		{
			name:        "agent with descriptive parent",
			logicalPath: "agents/researcher/AGENT.md",
			assetType:   "agent_definition",
			wantName:    "researcher",
		},
		{
			name:        "agent directly under generic agents dir",
			logicalPath: "agents/AGENT.md",
			assetType:   "agent_definition",
			wantName:    "AGENT.md",
		},
		{
			name:        ".claude/agents prefix",
			logicalPath: ".claude/agents/shaping-architect/AGENT.md",
			assetType:   "agent_definition",
			wantName:    "shaping-architect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := &client.BundleManifest{
				Layers: []client.BundleLayer{
					{LogicalPath: tt.logicalPath, AssetType: tt.assetType},
				},
			}

			summary := SummarizeBundleManifest(manifest)

			var got []string

			switch tt.assetType {
			case "skill":
				got = summary.Skills
			case "agent_definition":
				got = summary.Agents
			default:
				got = summary.Other
			}

			if len(got) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(got))
			}

			if got[0] != tt.wantName {
				t.Errorf("name = %q, want %q", got[0], tt.wantName)
			}
		})
	}
}

func TestDescriptiveAncestor(t *testing.T) {
	tests := []struct {
		logicalPath string
		fallback    string
		want        string
	}{
		{"skills/hello/SKILL.md", "SKILL.md", "hello"},
		{".claude/skills/hello/SKILL.md", "SKILL.md", "hello"},
		{"SKILL.md", "SKILL.md", "SKILL.md"},
		{"skills/SKILL.md", "SKILL.md", "SKILL.md"},
		{".claude/skills/SKILL.md", "SKILL.md", "SKILL.md"},
		{"agents/researcher/AGENT.md", "AGENT.md", "researcher"},
		{".claude/agents/researcher/AGENT.md", "AGENT.md", "researcher"},
		{"agents/AGENT.md", "AGENT.md", "AGENT.md"},
	}

	for _, tt := range tests {
		got := descriptiveAncestor(tt.logicalPath, tt.fallback)
		if got != tt.want {
			t.Errorf("descriptiveAncestor(%q, %q) = %q, want %q",
				tt.logicalPath, tt.fallback, got, tt.want)
		}
	}
}
