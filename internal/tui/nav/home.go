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

// renderHomeTwoPanel draws the stacked layout with menu and harness panel side-by-side, context below.
func renderHomeTwoPanel(mdl *model) string {
	menuActive := mdl.homeFocusArea == 0
	leftContent := renderMenuContent(mdl)
	leftTitle := renderPanelTitle(mdl)
	leftPanel := renderPanel(&mdl.styles, leftTitle, leftContent, mdl.styles.menuWidth, menuActive)

	// Add experimental panel below menu when enabled.
	if mdl.deps != nil && mdl.deps.Experimental {
		expPanel := renderExperimentalPanel(mdl, mdl.styles.menuWidth)
		leftPanel = lipgloss.JoinVertical(lipgloss.Center, leftPanel, "", expPanel)
	}

	// Build the top block: menu panel + optional harness panel side by side.
	// Both panels use the same width for visual alignment.
	var topPanels string

	hpWidth := mdl.styles.menuWidth
	if len(mdl.homeHarness.statuses) > 0 || mdl.homeHarness.loading {
		harnessActive := mdl.homeFocusArea == 1
		hPanel := renderHarnessPanel(mdl, hpWidth, harnessActive)
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", hPanel)
	} else {
		topPanels = leftPanel
	}

	footer := renderHomeFooter(mdl)

	// Context panel spans the full width of both panels + gap.
	ctxWidth := mdl.styles.menuWidth*2 + 2 //nolint:mnd // two panels + gap
	if maxW := mdl.width - 4; ctxWidth > maxW {
		ctxWidth = maxW
	}

	rightContent := renderContextContent(mdl, ctxWidth)
	musherTitle := hyperlink("https://musher.dev", "musher.dev")
	contextPanel := renderPanel(&mdl.styles, musherTitle, rightContent, ctxWidth, false)

	topBlock := lipgloss.JoinVertical(lipgloss.Center, topPanels, "", footer)

	// Center the mush panel vertically in the space above the bottom context panel.
	bottomMargin := 2
	bottomHeight := lipgloss.Height(contextPanel)
	topAreaHeight := mdl.height - bottomHeight - bottomMargin

	if topAreaHeight < lipgloss.Height(topBlock) {
		topAreaHeight = lipgloss.Height(topBlock)
	}

	centeredTop := lipgloss.Place(
		0, topAreaHeight,
		lipgloss.Center, lipgloss.Center,
		topBlock,
	)

	content := lipgloss.JoinVertical(lipgloss.Center, centeredTop, contextPanel)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Top,
		content,
	)
}

// renderHomeSinglePanel draws the single-panel layout with a collapsed context line.
func renderHomeSinglePanel(mdl *model) string {
	header := renderHeader(&mdl.styles, mdl.version(), mdl.updateAvailable)
	statusLine := renderStatusLine(mdl)
	menu := renderMenu(mdl)
	desc := renderDescription(mdl)
	footer := renderHomeFooter(mdl)

	parts := []string{
		header,
		"",
		statusLine,
		menu,
	}

	// Add experimental panel between menu and description when enabled.
	if mdl.deps != nil && mdl.deps.Experimental {
		expPanel := renderExperimentalPanel(mdl, mdl.styles.menuWidth)
		parts = append(parts, "", expPanel)
	}

	parts = append(parts, desc, "", footer)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		parts...,
	)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// formatVersionLabel formats a version string for display.
func formatVersionLabel(ver string) string {
	if ver == "dev" {
		return "dev"
	}

	return "v" + ver
}

// renderPanelTitle returns the two-panel left panel title including version and update indicator.
func renderPanelTitle(mdl *model) string {
	title := "mush " + formatVersionLabel(mdl.version())
	if mdl.updateAvailable {
		badge := lipgloss.NewStyle().Foreground(colorWarning).Render("update available")
		title += " · " + badge
	}

	return title
}

// renderHeader returns the brand logo line and tagline.
func renderHeader(styles *theme, ver string, updateAvailable bool) string {
	versionLabel := formatVersionLabel(ver)
	brand := styles.logo.Render("mush") + " " + styles.version.Render(versionLabel)

	if updateAvailable {
		badge := lipgloss.NewStyle().Foreground(colorWarning).Render("update available")
		brand += "  " + badge
	}

	tagline := styles.tagline.Render("Portable agent bundles")

	return lipgloss.JoinVertical(lipgloss.Center, brand, tagline)
}

// renderMenuContent renders the raw menu items (without box) for use inside a panel.
func renderMenuContent(mdl *model) string {
	var rows []string

	for idx, item := range mdl.items {
		if item.isSection {
			rows = append(rows, renderSectionHeader(mdl, idx, item))
		} else {
			rows = append(rows, renderMenuItem(mdl, idx, item))
		}
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Add description below items.
	if mdl.cursor >= 0 && mdl.cursor < len(mdl.items) && !mdl.items[mdl.cursor].isSection {
		desc := mdl.styles.description.Render(mdl.items[mdl.cursor].description)
		inner = lipgloss.JoinVertical(lipgloss.Left, inner, "", desc)
	}

	return inner
}

// renderMenu returns the cursor-highlighted menu items inside a rounded box.
func renderMenu(mdl *model) string {
	var rows []string

	for idx, item := range mdl.items {
		if item.isSection {
			rows = append(rows, renderSectionHeader(mdl, idx, item))
		} else {
			rows = append(rows, renderMenuItem(mdl, idx, item))
		}
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return mdl.styles.menuBox.Render(inner)
}

// renderSectionHeader renders a section divider label (e.g. "DEVELOP", "OPERATE").
func renderSectionHeader(mdl *model, idx int, item menuItem) string {
	header := mdl.styles.sectionHeader.Render(item.label)

	// Add blank line before all section headers except the first.
	if idx > 0 {
		return "\n" + header
	}

	return header
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

// renderContextContent renders the auth summary card.
func renderContextContent(mdl *model, panelWidth int) string {
	authTitle := mdl.styles.sectionTitle.Render("Auth")

	contentWidth := panelWidth - 6 //nolint:mnd // border(2)+padding(4)

	var bodyLines []string

	switch {
	case mdl.ctxInfo.loading:
		bodyLines = []string{
			mdl.styles.placeholder.Render("Loading..."),
		}
	case mdl.ctxInfo.authStatus == "authenticated":
		name := contextDisplayName(mdl)

		greeting := strings.TrimSpace(mdl.ctxInfo.greeting)
		if greeting == "" {
			greeting = "Welcome back"
		}

		if name != "" {
			bodyLines = append(bodyLines, mdl.styles.progressText.Render(greeting+", "+name))
		} else {
			bodyLines = append(bodyLines, mdl.styles.progressText.Render(greeting))
		}

		if mdl.ctxInfo.organizationName != "" {
			bodyLines = append(bodyLines, mdl.styles.placeholder.Render("Organization: "+mdl.ctxInfo.organizationName))
		}
	case mdl.ctxInfo.authStatus != "authenticated":
		bodyLines = []string{
			renderStatusDot(&mdl.styles.statusWarning, "Not authenticated"),
		}
	}

	body := authTitle + "\n" + strings.Join(bodyLines, "\n")

	// Social links row.
	sep := lipgloss.NewStyle().Foreground(colorMuted).Render(" · ")
	linkStyle := lipgloss.NewStyle().Foreground(colorAccent)
	links := hyperlink("https://discord.gg/SaVMzMgX2c", linkStyle.Render("Discord")) + sep +
		hyperlink("https://github.com/musher-dev", linkStyle.Render("GitHub")) + sep +
		hyperlink("https://x.com/musherdev", linkStyle.Render("X"))
	socialRow := lipgloss.PlaceHorizontal(contentWidth, lipgloss.Center, links)

	return body + "\n\n" + socialRow
}

// renderStatusLine renders a collapsed context line for single-panel mode.
func renderStatusLine(mdl *model) string {
	if mdl.ctxInfo.loading {
		return mdl.styles.placeholder.Render("Loading...")
	}

	var parts []string

	if mdl.ctxInfo.authStatus == "authenticated" {
		if greeting := strings.TrimSpace(mdl.ctxInfo.greeting); greeting != "" {
			if name := contextDisplayName(mdl); name != "" {
				parts = append(parts, mdl.styles.progressText.Render(greeting+", "+name))
			} else {
				parts = append(parts, mdl.styles.progressText.Render(greeting))
			}
		}
	}

	if mdl.ctxInfo.organizationName != "" {
		parts = append(parts, mdl.styles.progressText.Render(mdl.ctxInfo.organizationName))
	}

	if mdl.ctxInfo.authStatus != "authenticated" {
		parts = append(parts, renderStatusDot(&mdl.styles.statusWarning, "not authenticated"))
	}

	sep := mdl.styles.hintSep.Render(" \u00B7 ")

	return strings.Join(parts, sep)
}

func contextDisplayName(mdl *model) string {
	for _, value := range []string{
		mdl.ctxInfo.userFullName,
		mdl.ctxInfo.username,
		mdl.ctxInfo.credentialName,
	} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

// renderHomeFooter returns the key-hint bar for the home screen.
func renderHomeFooter(mdl *model) string {
	hints := []hint{
		navigationHint(mdl.keys.Up, mdl.keys.Down, "navigate"),
	}

	// Show tab hint when additional focus areas are available.
	hasHarness := mdl.styles.layout == layoutTwoPanel && len(mdl.homeHarness.statuses) > 0
	hasExperimental := len(mdl.experimentalPanel.items) > 0

	if hasHarness || hasExperimental {
		hints = append(hints, bindingHint(mdl.keys.Tab, "switch"))
	}

	hints = append(hints,
		bindingHint(mdl.keys.Select, "select"),
		bindingHint(mdl.keys.Status, "status"),
		bindingHint(mdl.keys.Quit, "quit"),
	)

	return renderKeyHints(&mdl.styles, hints)
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
