package nav

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// cursorActive is the prefix for the currently highlighted menu item.
const cursorActive = "\u276F " // ❯ followed by a space

// cursorBlank is the prefix for non-highlighted menu items (same width as cursorActive).
const cursorBlank = "  "

// renderHome composes the full home screen view.
func renderHome(mdl *model) string {
	header := renderHeader(&mdl.styles, mdl.version())
	menu := renderMenu(mdl)
	desc := renderDescription(mdl)
	footer := renderFooter(&mdl.styles)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		header,
		"",
		menu,
		desc,
		"",
		footer,
	)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHeader returns the brand logo line and tagline.
func renderHeader(styles *theme, ver string) string {
	versionLabel := ver
	if ver != "dev" {
		versionLabel = "v" + ver
	}

	brand := styles.logo.Render("mush") + " " + styles.version.Render(versionLabel)
	tagline := styles.tagline.Render("Local agent runtime")

	return lipgloss.JoinVertical(lipgloss.Center, brand, tagline)
}

// renderMenu returns the cursor-highlighted menu items inside a rounded box.
func renderMenu(mdl *model) string {
	var rows []string

	for idx, item := range mdl.items {
		var row string

		if idx == mdl.cursor {
			row = mdl.styles.menuItemActive.Render(cursorActive + item.label)
		} else {
			row = mdl.styles.menuItem.Render(cursorBlank + item.label)
		}

		rows = append(rows, row)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return mdl.styles.menuBox.Render(inner)
}

// renderDescription returns the dynamic description for the highlighted item.
func renderDescription(mdl *model) string {
	if mdl.cursor < 0 || mdl.cursor >= len(mdl.items) {
		return ""
	}

	return mdl.styles.description.Render(mdl.items[mdl.cursor].description)
}

// renderFooter returns the key-hint bar.
func renderFooter(styles *theme) string {
	hints := []struct {
		key  string
		desc string
	}{
		{key: "j/\u2193", desc: "navigate"},
		{key: "enter", desc: "select"},
		{key: "q", desc: "quit"},
	}

	sep := styles.hintSep.Render(" \u2022 ")

	var parts []string

	for _, hint := range hints {
		entry := styles.hintKey.Render(hint.key) + " " + styles.hintDesc.Render(hint.desc)
		parts = append(parts, entry)
	}

	return strings.Join(parts, sep)
}

// renderPlaceholder shows a centered "coming soon" screen for unimplemented items.
func renderPlaceholder(mdl *model) string {
	title := mdl.styles.placeholder.Render(mdl.placeholderText + " \u2014 coming soon")
	hint := mdl.styles.placeholderHint.Render("Press esc or enter to go back")

	content := lipgloss.JoinVertical(lipgloss.Center, title, "", hint)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}
