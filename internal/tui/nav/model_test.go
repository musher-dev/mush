package nav

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	if mdl.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.cursor)
	}

	if len(mdl.items) != 4 {
		t.Errorf("items = %d, want 4", len(mdl.items))
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

func updateModel(mdl *model, msg tea.Msg) *model {
	result, _ := mdl.Update(msg)

	return result.(*model)
}

func TestNavigateDown(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// Move down through all items.
	for idx := 0; idx < len(mdl.items)-1; idx++ {
		mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	}

	if mdl.cursor != len(mdl.items)-1 {
		t.Errorf("cursor = %d, want %d", mdl.cursor, len(mdl.items)-1)
	}

	// Pressing down again should not go past the last item.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.cursor != len(mdl.items)-1 {
		t.Errorf("cursor should clamp at %d, got %d", len(mdl.items)-1, mdl.cursor)
	}
}

func TestNavigateUp(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// Pressing up at 0 should stay at 0.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.cursor)
	}

	// Move down then back up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.cursor)
	}
}

func TestNavigateVimKeys(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// j moves down.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	if mdl.cursor != 1 {
		t.Errorf("j: cursor = %d, want 1", mdl.cursor)
	}

	// k moves up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	if mdl.cursor != 0 {
		t.Errorf("k: cursor = %d, want 0", mdl.cursor)
	}
}

func TestSelectShowsPlaceholder(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// Move to second item and select.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenPlaceholder {
		t.Errorf("activeScreen = %d, want screenPlaceholder", mdl.activeScreen)
	}

	if mdl.placeholderText != mdl.items[1].label {
		t.Errorf("placeholderText = %q, want %q", mdl.placeholderText, mdl.items[1].label)
	}
}

func TestEscReturnsHome(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// Move to item 2, select, then esc.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}

	// Cursor position should be preserved.
	if mdl.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (preserved)", mdl.cursor)
	}
}

func TestEnterReturnsFromPlaceholder(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	// Select first item, then press enter to go back.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

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

			mdl := newModel()
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

	mdl := newModel()

	// Enter placeholder screen.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	// q should quit even from placeholder.
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command from placeholder screen")
	}
}

func TestWindowResize(t *testing.T) {
	t.Parallel()

	mdl := newModel()

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

	mdl := newModel()

	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 100, Height: 40})

	if mdl.styles.menuWidth != menuWidthFull {
		t.Errorf("menuWidth = %d, want %d (full)", mdl.styles.menuWidth, menuWidthFull)
	}
}

func TestViewContainsBranding(t *testing.T) {
	t.Parallel()

	mdl := newModel()
	view := mdl.View()

	if !strings.Contains(view, "mush") {
		t.Error("view should contain 'mush' branding")
	}

	if !strings.Contains(view, "Local agent runtime") {
		t.Error("view should contain tagline")
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

	mdl := newModel()
	view := mdl.View()

	// First item description should be visible.
	if !strings.Contains(view, mdl.items[0].description) {
		t.Errorf("view should contain first item description %q", mdl.items[0].description)
	}
}

func TestPlaceholderView(t *testing.T) {
	t.Parallel()

	mdl := newModel()

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})
	view := mdl.View()

	if !strings.Contains(view, "coming soon") {
		t.Error("placeholder view should contain 'coming soon'")
	}

	if !strings.Contains(view, mdl.items[0].label) {
		t.Error("placeholder view should contain the selected item label")
	}

	if !strings.Contains(view, "esc") {
		t.Error("placeholder view should contain back hint")
	}
}
