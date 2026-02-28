package status

import (
	"strings"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/harness/state"
)

func TestRenderWorkerTopBarIncludesStatus(t *testing.T) {
	s := state.Snapshot{
		Width:       100,
		Height:      30,
		StatusLabel: "Starting...",
		Now:         time.Unix(10, 0),
	}

	out := Render(&s)

	if !strings.Contains(out, "MUSH") || !strings.Contains(out, "Starting") {
		t.Fatalf("render output missing top bar status: %q", out)
	}
}

func TestRenderSidebarIncludesBundleAndMCP(t *testing.T) {
	s := state.Snapshot{
		Width:          140,
		Height:         30,
		SidebarVisible: true,
		SidebarWidth:   36,
		BundleName:     "my-kit",
		BundleVer:      "1.2.3",
		BundleLayers:   4,
		BundleSkills:   []string{"SKILL.md"},
		BundleAgents:   []string{"agent.md"},
		MCPServers: []state.MCPServerStatus{
			{Name: "linear", Loaded: true, Authenticated: true},
		},
		Now: time.Now(),
	}

	out := Render(&s)

	if !strings.Contains(out, "my-kit v1.2.3") {
		t.Fatalf("render output missing bundle label: %q", out)
	}

	if !strings.Contains(out, "linear") {
		t.Fatalf("render output missing MCP row: %q", out)
	}
}
