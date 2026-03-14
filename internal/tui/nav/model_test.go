package nav

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel() *model {
	return newModel(context.Background(), nil)
}

func testModelExperimental() *model {
	return newModel(context.Background(), &Dependencies{Experimental: true})
}

func updateModel(mdl *model, msg tea.Msg) *model {
	result, _ := mdl.Update(msg)

	return result.(*model)
}

func TestNewModel(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	if mdl.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (skip first section header)", mdl.cursor)
	}

	if len(mdl.items) != 3 {
		t.Errorf("items = %d, want 3 (2 selectable + 1 section header)", len(mdl.items))
	}

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}

	if mdl.width != defaultWidth {
		t.Errorf("width = %d, want %d", mdl.width, defaultWidth)
	}

	if mdl.height != defaultHeight {
		t.Errorf("height = %d, want %d", mdl.height, defaultHeight)
	}
}

func TestNavigateDown(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Cursor starts at 1 (first selectable item, "Load bundle").
	// Down should go to 2 ("Find a bundle").
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (Find a bundle)", mdl.cursor)
	}

	// Pressing down again should not go past the last item.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.cursor != 2 {
		t.Errorf("cursor should clamp at 2, got %d", mdl.cursor)
	}
}

func TestExperimentalPanelNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Tab to focus experimental panel.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != homeFocusExperimental {
		t.Fatalf("focus = %d, want homeFocusExperimental", mdl.homeFocusArea)
	}

	// Cursor starts at 0 ("Start runner"). Down to 1 ("View history").
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.experimentalPanel.cursor != 1 {
		t.Errorf("experimentalPanel.cursor = %d, want 1", mdl.experimentalPanel.cursor)
	}

	// Down again should clamp.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.experimentalPanel.cursor != 1 {
		t.Errorf("experimentalPanel.cursor should clamp at 1, got %d", mdl.experimentalPanel.cursor)
	}

	// Up back to 0.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.experimentalPanel.cursor != 0 {
		t.Errorf("experimentalPanel.cursor = %d, want 0", mdl.experimentalPanel.cursor)
	}
}

func TestExperimentalPanelEnterActivates(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Tab to experimental panel, then enter on "Start runner" (cursor 0).
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	// Without auth, 'w' goes to worker error.
	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}
}

func TestExperimentalPanelEscReturnsFocus(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Tab to experimental panel.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.homeFocusArea != homeFocusExperimental {
		t.Fatalf("focus = %d, want homeFocusExperimental", mdl.homeFocusArea)
	}

	// Esc returns focus to menu.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.homeFocusArea != homeFocusMenu {
		t.Errorf("focus = %d, want homeFocusMenu after esc", mdl.homeFocusArea)
	}
}

func TestNavigateUp(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Cursor starts at 1. Pressing up should skip section header at 0, stay at 1.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (no selectable item above)", mdl.cursor)
	}

	// Move down to 2 ("Find a bundle"), then up to 1 ("Run harness").
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.cursor != 1 {
		t.Errorf("cursor = %d, want 1", mdl.cursor)
	}
}

func TestNavigateUpSkipsSection(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Cursor at 1 ("Load bundle"). Up should skip section header at 0, stay at 1.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (no selectable item above section header)", mdl.cursor)
	}
}

func TestNavigateVimKeys(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Cursor starts at 1. j moves down to 2.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if mdl.cursor != 2 {
		t.Errorf("j: cursor = %d, want 2", mdl.cursor)
	}

	// k moves up to 1.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	if mdl.cursor != 1 {
		t.Errorf("k: cursor = %d, want 1", mdl.cursor)
	}
}

func TestHotkeySelectsRunHarness(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// 'r' should jump to bundle input screen ("Run harness").
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Errorf("activeScreen = %d, want screenBundleInput", mdl.activeScreen)
	}

	if mdl.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (Run harness)", mdl.cursor)
	}
}

func TestHotkeyFindABundle(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if mdl.activeScreen != screenHubExplore {
		t.Errorf("activeScreen = %d, want screenHubExplore", mdl.activeScreen)
	}

	if mdl.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (Find a bundle)", mdl.cursor)
	}
}

func TestHotkeyCommaOpensSettings(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})

	if mdl.activeScreen != screenStatus {
		t.Errorf("activeScreen = %d, want screenStatus", mdl.activeScreen)
	}
}

func TestEscReturnsHome(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Navigate to "View history" via hotkey, then esc.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestEnterReturnsFromPlaceholder(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.placeholderText = "Test placeholder"
	mdl.pushScreen(screenPlaceholder)

	if mdl.activeScreen != screenPlaceholder {
		t.Fatalf("expected placeholder screen")
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after enter", mdl.activeScreen)
	}
}

func TestQuit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{name: "q", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
		{name: "ctrl+c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mdl := testModel()
			_, cmd := mdl.Update(test.msg)

			if cmd == nil {
				t.Fatal("expected quit command, got nil")
			}

			// Execute the command to verify it produces tea.QuitMsg.
			quitMsg := cmd()
			if _, ok := quitMsg.(tea.QuitMsg); !ok {
				t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
			}
		})
	}
}

func TestQuitFromPlaceholder(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenPlaceholder)

	// q should quit even from placeholder.
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command from placeholder screen")
	}
}

func TestWindowResize(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 50, Height: 30})

	if mdl.width != 50 {
		t.Errorf("width = %d, want 50", mdl.width)
	}

	if mdl.height != 30 {
		t.Errorf("height = %d, want 30", mdl.height)
	}

	if mdl.styles.menuWidth != menuWidthCompact {
		t.Errorf("menuWidth = %d, want %d (compact)", mdl.styles.menuWidth, menuWidthCompact)
	}
}

func TestWindowResizeFull(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 100, Height: 40})

	if mdl.styles.menuWidth != menuWidthFull {
		t.Errorf("menuWidth = %d, want %d (full)", mdl.styles.menuWidth, menuWidthFull)
	}
}

func TestViewContainsBranding(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	view := mdl.View()

	if !strings.Contains(view, "mush") {
		t.Error("view should contain 'mush' branding")
	}

	// Check all menu labels are present.
	for _, item := range mdl.items {
		if !strings.Contains(view, item.label) {
			t.Errorf("view should contain menu label %q", item.label)
		}
	}
}

func TestViewShowsActiveDescription(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	view := mdl.View()

	// First item description should be visible.
	if !strings.Contains(view, mdl.items[0].description) {
		t.Errorf("view should contain first item description %q", mdl.items[0].description)
	}
}

func TestPlaceholderView(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.placeholderText = "Test Feature"
	mdl.pushScreen(screenPlaceholder)

	view := mdl.View()

	if !strings.Contains(view, "coming soon") {
		t.Error("placeholder view should contain 'coming soon'")
	}

	if !strings.Contains(view, "Test Feature") {
		t.Error("placeholder view should contain the placeholder text")
	}

	if !strings.Contains(view, "esc") {
		t.Error("placeholder view should contain back hint")
	}
}

func TestScreenStack(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Push placeholder.
	mdl.pushScreen(screenPlaceholder)

	if mdl.activeScreen != screenPlaceholder {
		t.Errorf("activeScreen = %d, want screenPlaceholder", mdl.activeScreen)
	}

	if len(mdl.screenStack) != 1 {
		t.Errorf("screenStack len = %d, want 1", len(mdl.screenStack))
	}

	// Pop back to home.
	mdl.popScreen()

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after pop", mdl.activeScreen)
	}

	if len(mdl.screenStack) != 0 {
		t.Errorf("screenStack len = %d, want 0 after pop", len(mdl.screenStack))
	}
}

func TestPopEmptyStack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenPlaceholder

	// Pop with empty stack should go to home.
	mdl.popScreen()

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome on empty stack pop", mdl.activeScreen)
	}
}

func TestTwoPanelLayout(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 130, Height: 40})

	if mdl.styles.layout != layoutTwoPanel {
		t.Errorf("layout = %d, want layoutTwoPanel at width 130", mdl.styles.layout)
	}

	view := mdl.View()
	if !strings.Contains(view, "musher.dev") {
		t.Error("two-panel view should contain 'musher.dev' panel title")
	}
}

func TestSinglePanelLayout(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 70, Height: 40})

	if mdl.styles.layout != layoutSingle {
		t.Errorf("layout = %d, want layoutSingle at width 70", mdl.styles.layout)
	}

	view := mdl.View()
	if !strings.Contains(view, "mush") {
		t.Error("single-panel view should contain 'mush' branding")
	}
}

func TestMenuItemHotkeys(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Section headers have zero-value hotkey (0), selectable items have their hotkey.
	expected := []rune{0, 'r', 'f'}
	for idx, item := range mdl.items {
		if item.hotkey != expected[idx] {
			t.Errorf("item %d hotkey = %c, want %c", idx, item.hotkey, expected[idx])
		}
	}
}

func TestMenuItemHotkeysExperimental(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Main menu hotkeys are same as non-experimental.
	expectedMenu := []rune{0, 'r', 'f'}
	for idx, item := range mdl.items {
		if item.hotkey != expectedMenu[idx] {
			t.Errorf("menu item %d hotkey = %c, want %c", idx, item.hotkey, expectedMenu[idx])
		}
	}

	// Experimental panel hotkeys.
	expectedExp := []rune{'w', 'h'}
	for idx, item := range mdl.experimentalPanel.items {
		if item.hotkey != expectedExp[idx] {
			t.Errorf("experimental item %d hotkey = %c, want %c", idx, item.hotkey, expectedExp[idx])
		}
	}
}

func TestSectionHeadersAreMarked(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	if !mdl.items[0].isSection {
		t.Error("item 0 should be a section header (DEVELOP)")
	}

	if mdl.items[1].isSection {
		t.Error("item 1 should not be a section header (Load bundle)")
	}
}

func TestSectionHeadersExperimental(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Main menu still only has DEVELOP section (OPERATE items are in experimental panel).
	if !mdl.items[0].isSection {
		t.Error("item 0 should be a section header (DEVELOP)")
	}

	// Experimental panel should have 2 selectable items.
	if len(mdl.experimentalPanel.items) != 2 {
		t.Errorf("experimentalPanel.items = %d, want 2", len(mdl.experimentalPanel.items))
	}
}

func TestMenuItemsExperimental(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()

	// Main menu has same 3 items regardless of experimental.
	if len(mdl.items) != 3 {
		t.Errorf("items = %d, want 3", len(mdl.items))
	}

	// Experimental panel has 2 items.
	if len(mdl.experimentalPanel.items) != 2 {
		t.Errorf("experimentalPanel.items = %d, want 2", len(mdl.experimentalPanel.items))
	}
}

func TestHotkeyWorkerExperimental(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	// Without auth, 'w' should go to worker error screen.
	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}
}

func TestHotkeyHistoryExperimental(t *testing.T) {
	t.Parallel()

	mdl := testModelExperimental()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if mdl.activeScreen != screenHistory {
		t.Errorf("activeScreen = %d, want screenHistory", mdl.activeScreen)
	}
}

func TestContextInfoMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	msg := contextInfoMsg{
		authStatus:       "authenticated",
		organizationName: "test-org",
		organizationID:   "org-123",
	}

	mdl = updateModel(mdl, msg)

	if mdl.ctxInfo.loading {
		t.Error("ctxInfo.loading should be false after contextInfoMsg")
	}

	if mdl.ctxInfo.authStatus != "authenticated" {
		t.Errorf("authStatus = %q, want 'authenticated'", mdl.ctxInfo.authStatus)
	}

	if mdl.ctxInfo.organizationName != "test-org" {
		t.Errorf("organizationName = %q, want 'test-org'", mdl.ctxInfo.organizationName)
	}
}

func TestUpdateCheckMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	mdl = updateModel(mdl, updateCheckMsg{available: true, version: "9.9.9"})

	if !mdl.updateAvailable {
		t.Error("updateAvailable should be true after updateCheckMsg")
	}

	if mdl.updateVersion != "9.9.9" {
		t.Errorf("updateVersion = %q, want '9.9.9'", mdl.updateVersion)
	}
}

func TestUpdateCheckMsgNoUpdate(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	mdl = updateModel(mdl, updateCheckMsg{})

	if mdl.updateAvailable {
		t.Error("updateAvailable should be false when no update")
	}
}

func TestUpdateBadgeInTwoPanel(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.width = 130
	mdl.height = 40
	mdl.styles = newTheme(130)
	mdl.updateAvailable = true
	mdl.updateVersion = "9.9.9"

	view := mdl.View()

	if !strings.Contains(view, "update available") {
		t.Error("two-panel view should contain 'update available' badge")
	}
}

func TestUpdateBadgeInSinglePanel(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.width = 70
	mdl.styles = newTheme(70)
	mdl.updateAvailable = true
	mdl.updateVersion = "9.9.9"

	view := mdl.View()

	if !strings.Contains(view, "update available") {
		t.Error("single-panel view should contain 'update available' badge")
	}
}
