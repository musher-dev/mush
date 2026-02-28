package status

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/musher-dev/mush/internal/harness/state"
)

func TestTopBarLineNoBareResetExceptEnd(t *testing.T) {
	s := state.Snapshot{
		Width:       80,
		Height:      24,
		StatusLabel: "Ready",
	}

	line := topBarLine(&s)

	// The line must end with " \x1b[0m" (final full reset).
	const fullReset = "\x1b[0m"
	if !strings.HasSuffix(line, " "+fullReset) {
		t.Fatalf("topBarLine should end with %q, got suffix %q", " "+fullReset, line[max(0, len(line)-20):])
	}

	// Strip the trailing full reset, then verify no bare \x1b[0m remains.
	interior := line[:len(line)-len(" "+fullReset)]
	if strings.Contains(interior, fullReset) {
		t.Fatalf("topBarLine interior contains bare \\x1b[0m which resets the bar background;\nline = %q", line)
	}
}

func TestTopBarLineBackgroundPersists(t *testing.T) {
	for _, label := range []string{"Starting...", "Ready", "Connected", "Processing", "Error"} {
		t.Run(label, func(t *testing.T) {
			s := state.Snapshot{
				Width:       100,
				Height:      30,
				StatusLabel: label,
			}
			line := topBarLine(&s)

			const fullReset = "\x1b[0m"
			if !strings.HasSuffix(line, " "+fullReset) {
				t.Fatalf("topBarLine(%q) should end with %q, got suffix %q", label, " "+fullReset, line[max(0, len(line)-20):])
			}

			interior := line[:len(line)-len(" "+fullReset)]
			if strings.Contains(interior, fullReset) {
				t.Fatalf("topBarLine(%q) interior has bare \\x1b[0m;\nline = %q", label, line)
			}
		})
	}
}

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

func TestSidebarRowTruncatesCJKByCellWidth(t *testing.T) {
	// "你好世界测试" = 6 runes but 12 cells (each CJK char is 2 cells wide).
	// With sidebarWidth=10, maxContent=8. Truncating by cell width should
	// yield 4 CJK chars (8 cells), not 8 chars (which would overflow).
	content := "你好世界测试"
	sidebarWidth := 10

	row := sidebarRow(content, sidebarWidth)

	// The row should NOT contain the full 6-char string.
	if strings.Contains(row, "测试") {
		t.Fatalf("sidebarRow should have truncated CJK content by cell width, but got full string in: %q", row)
	}

	// Verify the visible content width fits within the sidebar.
	// Extract content between the sidebar background start and the border char.
	// The truncated content ("你好世界") is 4 runes / 8 cells, which fits maxContent=8.
	if !strings.Contains(row, "你好世界") {
		t.Fatalf("sidebarRow should contain first 4 CJK chars (8 cells), got: %q", row)
	}

	// Verify runewidth.StringWidth confirms the truncation is cell-aware.
	truncated := runewidth.Truncate(content, 8, "")
	if w := runewidth.StringWidth(truncated); w > 8 {
		t.Fatalf("truncated CJK string width = %d, want <= 8", w)
	}
}
