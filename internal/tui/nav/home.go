package nav

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// cursorActive is the prefix for the currently highlighted menu item.
const cursorActive = "\u276F " // ❯ followed by a space

// cursorBlank is the prefix for non-highlighted menu items (same width as cursorActive).
const cursorBlank = "  "

// renderHome composes the full home screen view.
func renderHome(mdl *model) string {
	if mdl.styles.layout == layoutTwoPanel {
		return renderHomeTwoPanel(mdl)
	}

	return renderHomeSinglePanel(mdl)
}

// renderHomeTwoPanel draws the two-panel layout with menu left and context right.
func renderHomeTwoPanel(mdl *model) string {
	leftContent := renderMenuContent(mdl)
	leftPanel := renderPanel(&mdl.styles, "mush", leftContent, mdl.styles.menuWidth, true)

	rightContent := renderContextContent(mdl)
	rightPanel := renderPanel(&mdl.styles, "musher.dev", rightContent, mdl.styles.contextWidth, false)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "   ", rightPanel)

	footer := renderHomeFooter(mdl)
	content := lipgloss.JoinVertical(lipgloss.Left, panels, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHomeSinglePanel draws the single-panel layout with a collapsed context line.
func renderHomeSinglePanel(mdl *model) string {
	header := renderHeader(&mdl.styles, mdl.version())
	statusLine := renderStatusLine(mdl)
	menu := renderMenu(mdl)
	desc := renderDescription(mdl)
	footer := renderHomeFooter(mdl)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		header,
		"",
		statusLine,
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

// renderMenuContent renders the raw menu items (without box) for use inside a panel.
func renderMenuContent(mdl *model) string {
	var rows []string

	for idx, item := range mdl.items {
		rows = append(rows, renderMenuItem(mdl, idx, item))
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Add description below items.
	if mdl.cursor >= 0 && mdl.cursor < len(mdl.items) {
		desc := mdl.styles.description.Render(mdl.items[mdl.cursor].description)
		inner = lipgloss.JoinVertical(lipgloss.Left, inner, "", desc)
	}

	return inner
}

// renderMenu returns the cursor-highlighted menu items inside a rounded box.
func renderMenu(mdl *model) string {
	var rows []string

	for idx, item := range mdl.items {
		rows = append(rows, renderMenuItem(mdl, idx, item))
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return mdl.styles.menuBox.Render(inner)
}

// renderMenuItem renders a single menu row: "❯ Label              [b]" or "  Label              [b]".
func renderMenuItem(mdl *model, idx int, item menuItem) string {
	active := idx == mdl.cursor
	prefix := cursorBlank

	if active {
		prefix = cursorActive
	}

	hotkeyStyle := mdl.styles.hotkey
	if active {
		hotkeyStyle = mdl.styles.hotkeyActive
	}

	// Hotkey badge in brackets: [b]
	hotkeyBadge := hotkeyStyle.Render(fmt.Sprintf("[%c]", item.hotkey))

	// Calculate available width for the label to right-align the hotkey badge.
	// Badge is 3 chars visual width: [x]
	// Item width = menuWidth - 6 (border + padding). Prefix=2, badge=3, gap=1.
	labelWidth := mdl.styles.menuWidth - 12 //nolint:mnd // border(2)+pad(4)+prefix(2)+badge(3)+gap(1)
	if labelWidth < 8 {
		labelWidth = 8
	}

	paddedLabel := item.label
	visualLen := lipgloss.Width(paddedLabel)

	if visualLen < labelWidth {
		paddedLabel += strings.Repeat(" ", labelWidth-visualLen)
	}

	content := prefix + paddedLabel + " " + hotkeyBadge
	if active {
		return mdl.styles.menuItemActive.Render(content)
	}

	return mdl.styles.menuItem.Render(content)
}

// renderDescription returns the dynamic description for the highlighted item.
func renderDescription(mdl *model) string {
	if mdl.cursor < 0 || mdl.cursor >= len(mdl.items) {
		return ""
	}

	return mdl.styles.description.Render(mdl.items[mdl.cursor].description)
}

// renderContextContent renders the context sidebar sections.
func renderContextContent(mdl *model) string {
	var sections []string

	// Workspace section.
	wsTitle := mdl.styles.sectionTitle.Render("Workspace")
	wsValue := mdl.styles.placeholder.Render("Loading...")

	if !mdl.ctxInfo.loading {
		if mdl.ctxInfo.workspaceName != "" {
			wsValue = mdl.styles.progressText.Render(mdl.ctxInfo.workspaceName)
		} else {
			wsValue = mdl.styles.placeholder.Render("not linked")
		}
	}

	sections = append(sections, wsTitle+"\n"+wsValue)

	// Auth section.
	authTitle := mdl.styles.sectionTitle.Render("Auth")
	authValue := mdl.styles.placeholder.Render("Loading...")

	if !mdl.ctxInfo.loading {
		if mdl.ctxInfo.authStatus == "authenticated" {
			authValue = renderStatusDot(&mdl.styles.statusOK, "Authenticated")
		} else {
			authValue = renderStatusDot(&mdl.styles.statusWarning, "Not authenticated")
		}
	}

	sections = append(sections, authTitle+"\n"+authValue)

	// Recent sessions section.
	recentTitle := mdl.styles.sectionTitle.Render("Recent jobs")

	switch {
	case mdl.ctxInfo.loading:
		sections = append(sections, recentTitle+"\n"+mdl.styles.placeholder.Render("Loading..."))
	case len(mdl.ctxInfo.recentSessions) == 0:
		sections = append(sections, recentTitle+"\n"+mdl.styles.placeholder.Render("no jobs yet"))
	default:
		var sessionLines []string

		for _, s := range mdl.ctxInfo.recentSessions {
			id := s.SessionID
			if len(id) > 7 {
				id = id[:7]
			}

			ago := formatTimeAgo(s.StartedAt)
			line := mdl.styles.progressText.Render(id) + "  " + mdl.styles.placeholder.Render(ago)
			sessionLines = append(sessionLines, line)
		}

		sessions := strings.Join(sessionLines, "\n")
		sections = append(sections, recentTitle+"\n"+sessions)
	}

	return strings.Join(sections, "\n\n")
}

// renderStatusLine renders a collapsed context line for single-panel mode.
func renderStatusLine(mdl *model) string {
	if mdl.ctxInfo.loading {
		return mdl.styles.placeholder.Render("Loading...")
	}

	var parts []string

	if mdl.ctxInfo.workspaceName != "" {
		parts = append(parts, mdl.styles.progressText.Render(mdl.ctxInfo.workspaceName))
	}

	if mdl.ctxInfo.authStatus == "authenticated" {
		parts = append(parts, renderStatusDot(&mdl.styles.statusOK, "authenticated"))
	} else {
		parts = append(parts, renderStatusDot(&mdl.styles.statusWarning, "not authenticated"))
	}

	sep := mdl.styles.hintSep.Render(" \u00B7 ")

	return strings.Join(parts, sep)
}

// renderHomeFooter returns the key-hint bar for the home screen.
func renderHomeFooter(mdl *model) string {
	return renderKeyHints(&mdl.styles, []hint{
		{key: "j/k", desc: "navigate"},
		{key: "enter", desc: "select"},
		{key: "q", desc: "quit"},
	})
}

// renderPlaceholder shows a centered "coming soon" screen for unimplemented items.
func renderPlaceholder(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", mdl.placeholderText})
	title := mdl.styles.placeholder.Render(mdl.placeholderText + " \u2014 coming soon")
	hintText := mdl.styles.placeholderHint.Render("Press esc to go back")

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", title, "", hintText)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// formatTimeAgo returns a human-friendly relative time string.
func formatTimeAgo(when time.Time) string {
	elapsed := time.Since(when)

	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		mins := int(elapsed.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	case elapsed < 24*time.Hour: //nolint:mnd // 24 hours in a day
		hours := int(elapsed.Hours())
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(elapsed.Hours() / 24) //nolint:mnd // 24 hours in a day
		return fmt.Sprintf("%dd ago", days)
	}
}
