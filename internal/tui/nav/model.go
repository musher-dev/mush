package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/buildinfo"
)

// screen identifies which screen is currently active.
type screen int

const (
	screenHome screen = iota
	screenPlaceholder
)

// menuItem represents a single entry in the home menu.
type menuItem struct {
	label       string
	description string
}

// model is the top-level Bubbletea model for the interactive TUI.
type model struct {
	width           int
	height          int
	cursor          int
	items           []menuItem
	activeScreen    screen
	placeholderText string
	keys            keyMap
	styles          theme
}

// defaultWidth is the assumed terminal width before the first WindowSizeMsg.
const defaultWidth = 80

// defaultHeight is the assumed terminal height before the first WindowSizeMsg.
const defaultHeight = 24

func newModel() *model {
	return &model{
		width:  defaultWidth,
		height: defaultHeight,
		items: []menuItem{
			{label: "Load a bundle", description: "Install and run a skill bundle"},
			{label: "Start worker", description: "Connect to a queue and process jobs"},
			{label: "View history", description: "Browse recent transcript sessions"},
			{label: "Check status", description: "Run connectivity diagnostics"},
		},
		activeScreen: screenHome,
		keys:         defaultKeyMap(),
		styles:       newTheme(defaultWidth),
	}
}

// Init satisfies tea.Model.
func (m *model) Init() tea.Cmd { return nil }

// Update handles messages and returns the updated model.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.styles = newTheme(msg.Width)

		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey dispatches key events to the active screen handler.
func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit bindings work on every screen.
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	switch m.activeScreen {
	case screenHome:
		return m.handleHomeKey(msg)
	case screenPlaceholder:
		return m.handlePlaceholderKey(msg)
	}

	return m, nil
}

// handleHomeKey processes key events on the home screen.
func (m *model) handleHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, m.keys.Select):
		m.placeholderText = m.items[m.cursor].label
		m.activeScreen = screenPlaceholder
	}

	return m, nil
}

// handlePlaceholderKey processes key events on the placeholder screen.
func (m *model) handlePlaceholderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Select) {
		m.activeScreen = screenHome
	}

	return m, nil
}

// View renders the current screen.
func (m *model) View() string {
	switch m.activeScreen {
	case screenPlaceholder:
		return renderPlaceholder(m)
	default:
		return renderHome(m)
	}
}

// version returns the build version string for display.
func (m *model) version() string {
	return buildinfo.Version
}
