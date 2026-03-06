//go:build unix

package harness

import (
	"strings"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/harness/ui/layout"
)

func TestEmbeddedSidebarLinesIncludeBundleAssetSections(t *testing.T) {
	r := &embeddedRuntime{
		cfg:                config.Load(),
		supportedHarnesses: []string{"claude"},
		frame:              layout.ComputeFrame(140, 40, true),
		width:              140,
		height:             40,
		bundleLoadMode:     true,
		bundleName:         "readme-maker",
		bundleVer:          "0.1.0",
		bundleSummary: BundleSummary{
			TotalLayers: 5,
			Agents:      []string{"layout-constructor.md"},
			Skills:      []string{"README.md"},
			ToolConfigs: []string{"mcp.json"},
			Other:       []string{"notes.txt"},
		},
		now: time.Now,
		jobs: &JobLoop{
			status:        StatusReady,
			lastHeartbeat: time.Now(),
		},
	}

	lines := r.sidebarLines(layout.PtyRowsForFrame(r.frame))
	joined := strings.Join(lines, "\n")

	for _, want := range []string{
		"Bundle",
		"readme-maker v0.1.0",
		"layers: 5",
		"agents: 1",
		"skills: 1",
		"tools: 1",
		"other: 1",
		"Agents",
		"layout-constructor.md",
		"Skills",
		"README.md",
		"Tools",
		"mcp.json",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("sidebar lines missing %q\n%s", want, joined)
		}
	}
}
