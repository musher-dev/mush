package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// renderExperimentalPanel renders the experimental panel with navigable menu items.
func renderExperimentalPanel(mdl *model, width int) string {
	if len(mdl.experimentalPanel.items) == 0 {
		body := mdl.styles.placeholder.Render("No experimental features are currently available.")
		return renderPanel(&mdl.styles, "Experimental", body, width, false)
	}

	active := mdl.homeFocusArea == homeFocusExperimental

	header := mdl.styles.sectionHeader.Render("OPERATE")

	rows := []string{header}

	for idx, item := range mdl.experimentalPanel.items {
		selected := active && idx == mdl.experimentalPanel.cursor
		rows = append(rows, renderExperimentalItem(mdl, item, width, selected))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Show description of selected item when panel is focused.
	if active {
		cur := mdl.experimentalPanel.cursor
		if cur >= 0 && cur < len(mdl.experimentalPanel.items) {
			desc := mdl.styles.description.Render(mdl.experimentalPanel.items[cur].description)
			body = lipgloss.JoinVertical(lipgloss.Left, body, "", desc)
		}
	}

	return renderPanel(&mdl.styles, "Experimental", body, width, active)
}

// renderExperimentalItem renders a single item in the experimental panel.
func renderExperimentalItem(mdl *model, item menuItem, panelWidth int, selected bool) string {
	prefix := cursorBlank
	if selected {
		prefix = cursorActive
	}

	hotkeyStyle := mdl.styles.hotkey
	if selected {
		hotkeyStyle = mdl.styles.hotkeyActive
	}

	hotkeyBadge := hotkeyStyle.Render(fmt.Sprintf("[%c]", item.hotkey))

	// Match the same width math as renderMenuItem.
	labelWidth := panelWidth - 12 //nolint:mnd // border(2)+pad(4)+prefix(2)+badge(3)+gap(1)
	if labelWidth < 8 {
		labelWidth = 8
	}

	paddedLabel := item.label
	if visualLen := lipgloss.Width(paddedLabel); visualLen < labelWidth {
		paddedLabel += strings.Repeat(" ", labelWidth-visualLen)
	}

	content := prefix + paddedLabel + " " + hotkeyBadge
	if selected {
		return mdl.styles.menuItemActive.Render(content)
	}

	return mdl.styles.menuItem.Render(content)
}

// handleExperimentalPanelKey processes key events when the experimental panel is focused.
func (m *model) handleExperimentalPanelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.experimentalPanel.cursor < len(m.experimentalPanel.items)-1 {
			m.experimentalPanel.cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.experimentalPanel.cursor > 0 {
			m.experimentalPanel.cursor--
		}

	case key.Matches(msg, m.keys.Select):
		if m.experimentalPanel.cursor >= 0 && m.experimentalPanel.cursor < len(m.experimentalPanel.items) {
			return m.activateByHotkey(m.experimentalPanel.items[m.experimentalPanel.cursor].hotkey)
		}

	case key.Matches(msg, m.keys.Back):
		m.homeFocusArea = homeFocusMenu
	}

	return m, nil
}
