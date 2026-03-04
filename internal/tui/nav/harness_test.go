package nav

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/harness"
)

func testModelWithHarnesses() *model {
	mdl := testModel()
	mdl.width = 120
	mdl.height = 40
	mdl.styles = newTheme(120)
	mdl.homeHarness = homeHarnessState{
		expanded: -1,
		loading:  false,
		statuses: []harnessQuickStatus{
			{name: "claude", displayName: "Claude Code", installed: true, version: "1.0.24"},
			{name: "codex", displayName: "Codex CLI", installed: false},
		},
	}

	return mdl
}

func TestHarnessPanelRendersInTwoPanel(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	view := mdl.View()

	if !strings.Contains(view, "Available harnesses") {
		t.Error("two-panel view should contain 'Available harnesses' panel title")
	}

	if !strings.Contains(view, "Claude Code") {
		t.Error("view should contain 'Claude Code' harness")
	}

	if !strings.Contains(view, "Codex CLI") {
		t.Error("view should contain 'Codex CLI' harness")
	}
}

func TestHarnessPanelHiddenInSinglePanel(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.width = 70
	mdl.styles = newTheme(70)

	view := mdl.View()

	if strings.Contains(view, "Available harnesses") {
		t.Error("single-panel view should not contain 'Available harnesses' panel")
	}
}

func TestHarnessPanelHiddenWhenNoStatuses(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeHarness.statuses = nil

	view := mdl.View()

	if strings.Contains(view, "Available harnesses") {
		t.Error("view should not contain 'Available harnesses' panel when no statuses")
	}
}

func TestTabTogglesFocus(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()

	if mdl.homeFocusArea != 0 {
		t.Fatalf("initial focus should be 0 (menu), got %d", mdl.homeFocusArea)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != 1 {
		t.Errorf("after tab, focus should be 1 (harness), got %d", mdl.homeFocusArea)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != 0 {
		t.Errorf("after second tab, focus should be 0 (menu), got %d", mdl.homeFocusArea)
	}
}

func TestTabNoOpWithoutHarnesses(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeHarness.statuses = nil

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != 0 {
		t.Errorf("tab should not switch focus when no harnesses, got %d", mdl.homeFocusArea)
	}
}

func TestHarnessCursorNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1

	if mdl.homeHarness.cursor != 0 {
		t.Fatalf("initial cursor should be 0, got %d", mdl.homeHarness.cursor)
	}

	// Move down.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.homeHarness.cursor != 1 {
		t.Errorf("cursor after down should be 1, got %d", mdl.homeHarness.cursor)
	}

	// Should clamp at last item.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.homeHarness.cursor != 1 {
		t.Errorf("cursor should clamp at 1, got %d", mdl.homeHarness.cursor)
	}

	// Move up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.homeHarness.cursor != 0 {
		t.Errorf("cursor after up should be 0, got %d", mdl.homeHarness.cursor)
	}

	// Should clamp at 0.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.homeHarness.cursor != 0 {
		t.Errorf("cursor should clamp at 0, got %d", mdl.homeHarness.cursor)
	}
}

func TestHarnessExpansionToggle(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1

	if mdl.homeHarness.expanded != -1 {
		t.Fatalf("initial expanded should be -1, got %d", mdl.homeHarness.expanded)
	}

	// Enter expands.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.homeHarness.expanded != 0 {
		t.Errorf("expanded should be 0 after enter, got %d", mdl.homeHarness.expanded)
	}

	if !mdl.harnessExpand.loading {
		t.Error("health check should be loading after expansion")
	}

	// Enter again collapses.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.homeHarness.expanded != -1 {
		t.Errorf("expanded should be -1 after second enter, got %d", mdl.homeHarness.expanded)
	}
}

func TestHarnessEscCollapsesExpansion(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.expanded = 0
	mdl.harnessExpand.loading = false
	mdl.harnessExpand.report = &harness.HealthReport{}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.homeHarness.expanded != -1 {
		t.Errorf("expanded should be -1 after esc, got %d", mdl.homeHarness.expanded)
	}

	// Focus should stay on harness panel.
	if mdl.homeFocusArea != 1 {
		t.Errorf("focus should stay on harness panel after esc collapse, got %d", mdl.homeFocusArea)
	}
}

func TestHarnessEscWithoutExpansionReturnsFocus(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.homeFocusArea != 0 {
		t.Errorf("focus should return to menu after esc without expansion, got %d", mdl.homeFocusArea)
	}
}

func TestHarnessInstallAction(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.cursor = 1 // Codex CLI (not installed)
	mdl.homeHarness.expanded = 1

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if mdl.result == nil {
		t.Fatal("result should be set after install")
	}

	if mdl.result.Action != ActionHarnessInstall {
		t.Errorf("action = %d, want ActionHarnessInstall", mdl.result.Action)
	}

	if len(mdl.result.InstallCommands) == 0 {
		t.Error("install commands should not be empty")
	}

	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestHarnessInstallNoOpWhenInstalled(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.cursor = 0 // Claude Code (installed)
	mdl.homeHarness.expanded = 0

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if mdl.result != nil {
		t.Error("install should be no-op when harness is installed")
	}
}

func TestHarnessInstallNoOpWhenNotExpanded(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.cursor = 1

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if mdl.result != nil {
		t.Error("install should be no-op when harness is not expanded")
	}
}

func TestHarnessStatusesLoadedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.homeHarness.loading = true

	statuses := []harnessQuickStatus{
		{name: "claude", displayName: "Claude Code", installed: true, version: "1.0.24"},
	}

	mdl = updateModel(mdl, harnessStatusesLoadedMsg{statuses: statuses})

	if mdl.homeHarness.loading {
		t.Error("loading should be false after status load")
	}

	if len(mdl.homeHarness.statuses) != 1 {
		t.Errorf("statuses count = %d, want 1", len(mdl.homeHarness.statuses))
	}
}

func TestHarnessExpandHealthMsg(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.expanded = 0
	mdl.harnessExpand.loading = true

	report := &harness.HealthReport{
		ProviderName: "claude",
		DisplayName:  "Claude Code",
		Results: []harness.HealthResult{
			{Check: "Binary", Message: "claude found", Status: harness.HealthPass},
		},
	}

	mdl = updateModel(mdl, harnessExpandHealthMsg{report: report})

	if mdl.harnessExpand.loading {
		t.Error("loading should be false after health report")
	}

	if mdl.harnessExpand.report == nil {
		t.Error("report should be set")
	}

	if mdl.harnessExpand.report.ProviderName != "claude" {
		t.Errorf("report provider = %q, want 'claude'", mdl.harnessExpand.report.ProviderName)
	}
}

func TestHarnessExpandHealthMsgIgnoredOnWrongScreen(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.activeScreen = screenPlaceholder
	mdl.harnessExpand.loading = true

	mdl = updateModel(mdl, harnessExpandHealthMsg{report: &harness.HealthReport{}})

	if !mdl.harnessExpand.loading {
		t.Error("should not update loading when on wrong screen")
	}
}

func TestHarnessVersionDisplay(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()

	view := mdl.View()

	if !strings.Contains(view, "v1.0.24") {
		t.Error("view should contain version 'v1.0.24' for installed harness")
	}

	if !strings.Contains(view, "not installed") {
		t.Error("view should contain 'not installed' for missing harness")
	}
}

func TestHarnessExpandedViewShowsHealthResults(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.expanded = 0
	mdl.harnessExpand.loading = false
	mdl.harnessExpand.report = &harness.HealthReport{
		ProviderName: "claude",
		DisplayName:  "Claude Code",
		Results: []harness.HealthResult{
			{Check: "Binary", Message: "claude found at /usr/local/bin/claude", Status: harness.HealthPass},
			{Check: "Version", Message: "1.0.24 (Claude Code)", Status: harness.HealthPass},
		},
	}

	view := mdl.View()

	if !strings.Contains(view, "Binary") {
		t.Error("expanded view should contain 'Binary' check")
	}

	if !strings.Contains(view, "/usr/local/bin/claude") {
		t.Error("expanded view should show binary path")
	}

	// Should show cleaned version without parenthetical.
	if !strings.Contains(view, "1.0.24") {
		t.Error("expanded view should contain version number")
	}

	if strings.Contains(view, "(Claude Code)") {
		t.Error("expanded view should not show parenthetical suffix in version")
	}

	// Should not show verbose "found at" prefix.
	if strings.Contains(view, "claude found at") {
		t.Error("expanded view should not show 'found at' prefix")
	}
}

func TestCleanHealthMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check   string
		message string
		want    string
	}{
		{"Binary", "claude found at /usr/local/bin/claude", "/usr/local/bin/claude"},
		{"Binary", "claude not found in PATH", "claude not found in PATH"},
		{"Version", "2.1.68 (Claude Code)", "2.1.68"},
		{"Version", "0.107.0", "0.107.0"},
		{"Config", "~/.claude", "~/.claude"},
	}

	for _, tt := range tests {
		got := cleanHealthMessage(tt.check, tt.message)
		if got != tt.want {
			t.Errorf("cleanHealthMessage(%q, %q) = %q, want %q", tt.check, tt.message, got, tt.want)
		}
	}
}

func TestMenuHotkeysWorkFromHarnessFocus(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1

	// 'r' should still activate bundle input (Run harness) even when harness panel is focused.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Errorf("activeScreen = %d, want screenBundleInput", mdl.activeScreen)
	}
}

func TestPopScreenResetsHarnessState(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.expanded = 0
	mdl.harnessExpand.loading = true

	mdl.pushScreen(screenPlaceholder)
	mdl.popScreen()

	if mdl.activeScreen != screenHome {
		t.Fatalf("should be on home screen, got %d", mdl.activeScreen)
	}

	if mdl.homeFocusArea != 0 {
		t.Errorf("focus should reset to 0, got %d", mdl.homeFocusArea)
	}

	if mdl.homeHarness.expanded != -1 {
		t.Errorf("expanded should reset to -1, got %d", mdl.homeHarness.expanded)
	}

	if mdl.harnessExpand.loading {
		t.Error("loading should be reset")
	}
}

func TestFooterShowsTabHint(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	view := mdl.View()

	if !strings.Contains(view, "tab") {
		t.Error("two-panel view with harnesses should contain 'tab' hint")
	}
}

func TestFooterNoTabHintWithoutHarnesses(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeHarness.statuses = nil

	footer := renderHomeFooter(mdl)

	if strings.Contains(footer, "tab") {
		t.Error("footer should not contain 'tab' hint without harnesses")
	}
}

func TestHarnessQuickStatusVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"1.0.24", "1.0.24"},
		{"claude v1.0.24", "v1.0.24"},
		{"codex 0.1.0", "0.1.0"},
		{"v2.0.0", "v2.0.0"},
		{"Claude Code 2.1.68 (Claude Code)", "2.1.68"},
		{"codex-cli 0.106.0", "0.106.0"},
	}

	for _, tt := range tests {
		got := harnessQuickStatusVersion(tt.input)
		if got != tt.want {
			t.Errorf("harnessQuickStatusVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTabCollapsesExpansionOnFocusSwitch(t *testing.T) {
	t.Parallel()

	mdl := testModelWithHarnesses()
	mdl.homeFocusArea = 1
	mdl.homeHarness.expanded = 0
	mdl.harnessExpand.report = &harness.HealthReport{}

	// Tab should switch focus back to menu and collapse expansion.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != 0 {
		t.Errorf("focus should be 0 after tab, got %d", mdl.homeFocusArea)
	}

	if mdl.homeHarness.expanded != -1 {
		t.Errorf("expanded should be -1 after tab away, got %d", mdl.homeHarness.expanded)
	}
}
