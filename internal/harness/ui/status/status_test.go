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

func TestTopBarShowsKeyboardHints(t *testing.T) {
	s := state.Snapshot{
		Width:       120,
		Height:      30,
		StatusLabel: "Ready",
	}

	line := topBarLine(&s)

	for _, hint := range []string{"^C Int", "^S Copy", "^Q Quit"} {
		if !strings.Contains(line, hint) {
			t.Fatalf("topBarLine missing hint %q in: %q", hint, line)
		}
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

func TestDistributeListSlots_StandardTerminal(t *testing.T) {
	lists := []listInfo{
		{"Agents", make([]string, 8)},
		{"Skills", make([]string, 4)},
		{"Tools", make([]string, 2)},
	}

	slots := distributeListSlots(lists, nil, 7, 6, 40)
	total := slots[0] + slots[1] + slots[2]

	if total == 0 {
		t.Fatal("expected some slots allocated")
	}

	// Agents has the most items, so should get the most slots.
	if slots[0] < slots[1] {
		t.Fatalf("agents (%d items) should get >= slots than skills (%d items): got %d vs %d",
			8, 4, slots[0], slots[1])
	}
}

func TestDistributeListSlots_TallTerminal(t *testing.T) {
	lists := []listInfo{
		{"Agents", make([]string, 3)},
		{"Skills", make([]string, 3)},
		{"Tools", make([]string, 3)},
	}

	// With 60 rows, all 9 items should fit without truncation.
	slots := distributeListSlots(lists, nil, 7, 6, 60)

	for i, li := range lists {
		if slots[i] < len(li.items) {
			t.Fatalf("list %q: expected all %d items to fit, got %d slots", li.title, len(li.items), slots[i])
		}
	}
}

func TestDistributeListSlots_ShortTerminal(t *testing.T) {
	lists := []listInfo{
		{"Agents", make([]string, 8)},
		{"Skills", make([]string, 4)},
		{"Tools", make([]string, 2)},
	}

	// With only 10 rows total, very few slots are available.
	slots := distributeListSlots(lists, nil, 7, 6, 10)

	// Should not exceed available items.
	for i, li := range lists {
		if slots[i] > len(li.items) {
			t.Fatalf("list %q: slots %d > items %d", li.title, slots[i], len(li.items))
		}
	}
}

func TestDistributeListSlots_EmptyLists(t *testing.T) {
	lists := []listInfo{
		{"Agents", nil},
		{"Skills", nil},
		{"Tools", nil},
	}

	slots := distributeListSlots(lists, nil, 7, 6, 40)

	for i := range slots {
		if slots[i] != 0 {
			t.Fatalf("empty list %d should get 0 slots, got %d", i, slots[i])
		}
	}
}

func TestDistributeListSlots_SingleList(t *testing.T) {
	lists := []listInfo{
		{"Agents", make([]string, 5)},
		{"Skills", nil},
		{"Tools", nil},
	}

	slots := distributeListSlots(lists, nil, 7, 6, 40)

	if slots[0] != 5 {
		t.Fatalf("single non-empty list should get all items, got %d", slots[0])
	}
}

func TestSidebarLines_ClickTargets(t *testing.T) {
	s := &state.Snapshot{
		BundleName:   "test",
		BundleVer:    "1.0",
		BundleLayers: 1,
		BundleAgents: []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11", "a12"},
		BundleSkills: []string{"s1", "s2", "s3", "s4", "s5", "s6"},
		BundleTools:  []string{"t1", "t2", "t3", "t4"},
		Now:          time.Now(),
	}

	// Use a short terminal so not all items fit.
	lines, targets := SidebarLines(s, 24)
	joined := strings.Join(lines, "\n")

	// Should have a "+N more (click)" line for agents.
	if !strings.Contains(joined, "more (click)") {
		t.Fatalf("expected '+N more (click)' in sidebar, got:\n%s", joined)
	}

	// Should have at least one click target for agents.
	found := false

	for _, tgt := range targets {
		if tgt.Section == "Agents" {
			found = true

			break
		}
	}

	if !found {
		t.Fatal("expected click target for Agents section")
	}
}

func TestSidebarLines_ExpandedSection(t *testing.T) {
	agents := []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}

	s := &state.Snapshot{
		BundleName:       "test",
		BundleVer:        "1.0",
		BundleLayers:     1,
		BundleAgents:     agents,
		BundleSkills:     []string{"s1"},
		ExpandedSections: map[string]bool{"Agents": true},
		Now:              time.Now(),
	}

	lines, targets := SidebarLines(s, 40)
	joined := strings.Join(lines, "\n")

	// All agents should be visible when expanded.
	for _, a := range agents {
		if !strings.Contains(joined, a) {
			t.Fatalf("expanded section missing agent %q in:\n%s", a, joined)
		}
	}

	// Should have [collapse] text.
	if !strings.Contains(joined, "[collapse]") {
		t.Fatalf("expected [collapse] in expanded section, got:\n%s", joined)
	}

	// Should have a collapse click target.
	found := false

	for _, tgt := range targets {
		if tgt.Section == "Agents" {
			found = true

			break
		}
	}

	if !found {
		t.Fatal("expected click target for collapsing Agents section")
	}
}

func TestSidebarLines_ClickTargetRowIndex(t *testing.T) {
	s := &state.Snapshot{
		BundleName:   "test",
		BundleVer:    "1.0",
		BundleLayers: 1,
		BundleAgents: []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"},
		BundleSkills: []string{"s1", "s2", "s3", "s4", "s5"},
		Now:          time.Now(),
	}

	lines, targets := SidebarLines(s, 20)

	for _, tgt := range targets {
		if tgt.Row < 0 || tgt.Row >= len(lines) {
			t.Fatalf("click target row %d out of bounds (lines len=%d)", tgt.Row, len(lines))
		}

		line := lines[tgt.Row]
		if !strings.Contains(line, "more (click)") && !strings.Contains(line, "[collapse]") {
			t.Fatalf("click target row %d doesn't contain expected text: %q", tgt.Row, line)
		}
	}
}
