package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// bundleHarnessPanelWidth is the fixed width for the right-hand harness panel in two-panel mode.
// Matches menuWidthFull to avoid wrapping long harness descriptions.
const bundleHarnessPanelWidth = 60

// renderBundleInput renders the bundle slug input screen.
// Wide terminals (>= layoutTwoPanel) get side-by-side panels; narrow terminals get the original single panel.
func renderBundleInput(mdl *model) string {
	if mdl.styles.layout >= layoutTwoPanel {
		return renderBundleInputTwoPanel(mdl)
	}

	return renderBundleInputSinglePanel(mdl)
}

// renderBundleInputTwoPanel draws two side-by-side panels: "Run harness" (left) and "Harness" (right).
func renderBundleInputTwoPanel(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness"})

	leftBody := renderBundleInputBody(mdl)
	leftActive := mdl.bundleInput.focusArea != bundleFocusHarness
	leftPanel := renderPanel(&mdl.styles, "Run harness", leftBody, mdl.styles.menuWidth, leftActive)

	// Right panel: harness list.
	rightBody := renderHarnessList(mdl)
	rightPanel := renderPanel(&mdl.styles, "Harness", rightBody, bundleHarnessPanelWidth, mdl.bundleInput.focusArea == bundleFocusHarness)

	// Align panel heights so the horizontal join looks clean.
	leftH := lipgloss.Height(leftPanel)
	rightH := lipgloss.Height(rightPanel)

	if leftH > rightH {
		rightPanel = lipgloss.PlaceVertical(leftH, lipgloss.Top, rightPanel)
	} else if rightH > leftH {
		leftPanel = lipgloss.PlaceVertical(rightH, lipgloss.Top, leftPanel)
	}

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch panel"},
		{key: "j/k", desc: "navigate"},
		{key: "enter", desc: "select"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panels, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleInputSinglePanel draws the original combined panel for narrow terminals.
func renderBundleInputSinglePanel(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness"})

	leftBody := renderBundleInputBody(mdl)

	harnessTitle := mdl.styles.sectionTitle.Render("Harness")
	harnessList := renderHarnessList(mdl)

	focusHint := mdl.styles.placeholder.Render("tab to switch focus")

	body := lipgloss.JoinVertical(lipgloss.Left,
		leftBody,
		"",
		harnessTitle,
		harnessList,
		"",
		focusHint,
	)

	panel := renderPanel(&mdl.styles, "Run harness", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch focus"},
		{key: "j/k", desc: "navigate"},
		{key: "enter", desc: "select"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// bundleListLen returns the total number of selectable items in the bundle list
// (recent + installed + 2 action links at the bottom).
func bundleListLen(mdl *model) int {
	return len(mdl.bundleInput.recentBundles) + len(mdl.bundleInput.installedBundles) + 2 //nolint:mnd // 2 action links
}

// renderBundleInputBody renders the shared left-panel body for both layout modes.
func renderBundleInputBody(mdl *model) string {
	// Text input.
	input := mdl.bundleInput.textInput.View()

	parts := []string{input}

	// Recent bundles.
	recentTitle := mdl.styles.sectionHeader.Render("RECENT")
	listIdx := 0

	if len(mdl.bundleInput.recentBundles) > 0 {
		var rows []string

		for i, r := range mdl.bundleInput.recentBundles {
			active := mdl.bundleInput.focusArea == bundleFocusList && mdl.bundleInput.listCursor == listIdx+i
			ref := r.namespace + "/" + r.slug + " v" + r.version
			row := renderBundleListRow(mdl, ref, r.timeAgo, active)
			rows = append(rows, row)
		}

		parts = append(parts, "", recentTitle, strings.Join(rows, "\n"))
		listIdx += len(mdl.bundleInput.recentBundles)
	} else if mdl.bundleInput.listLoaded {
		parts = append(parts, "", recentTitle, mdl.styles.placeholder.Render("no recent bundles"))
	}

	// Installed bundles.
	if len(mdl.bundleInput.installedBundles) > 0 {
		installedTitle := mdl.styles.sectionHeader.Render("INSTALLED")

		var rows []string

		for i, b := range mdl.bundleInput.installedBundles {
			active := mdl.bundleInput.focusArea == bundleFocusList && mdl.bundleInput.listCursor == listIdx+i

			label := b.slug
			if b.version != "" {
				label += " v" + b.version
			}

			detail := b.harness
			row := renderBundleListRow(mdl, label, detail, active)
			rows = append(rows, row)
		}

		parts = append(parts, "", installedTitle, strings.Join(rows, "\n"))
		listIdx += len(mdl.bundleInput.installedBundles)
	}

	// Action links: "Find a bundle on the Hub" and "Run without a bundle".
	sep := mdl.styles.placeholder.Render("── ── ── ── ── ── ──")
	findActive := mdl.bundleInput.focusArea == bundleFocusList && mdl.bundleInput.listCursor == listIdx
	findLink := renderBundleActionLink(mdl, "Find a bundle on the Hub", 'f', findActive)
	bareActive := mdl.bundleInput.focusArea == bundleFocusList && mdl.bundleInput.listCursor == listIdx+1
	bareLink := renderBundleActionLink(mdl, "Run without a bundle", 'n', bareActive)

	parts = append(parts, "", sep, findLink, bareLink)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderBundleListRow renders a single row in the recent/installed list.
func renderBundleListRow(mdl *model, label, detail string, active bool) string {
	prefix := cursorBlank
	style := mdl.styles.menuItem

	if active {
		prefix = cursorActive
		style = mdl.styles.menuItemActive
	}

	labelWidth := mdl.styles.menuWidth - 16 //nolint:mnd // border+pad+prefix+detail+gap
	if labelWidth < 8 {
		labelWidth = 8
	}

	padded := label
	if lipgloss.Width(padded) < labelWidth {
		padded += strings.Repeat(" ", labelWidth-lipgloss.Width(padded))
	}

	return style.Render(prefix + padded + " " + mdl.styles.placeholder.Render(detail))
}

// renderBundleActionLink renders a selectable link at the bottom of the Run screen.
func renderBundleActionLink(mdl *model, label string, hotkey rune, active bool) string {
	prefix := cursorBlank
	style := mdl.styles.menuItem

	if active {
		prefix = cursorActive
		style = mdl.styles.menuItemActive
	}

	hotkeyBadge := mdl.styles.hotkey.Render(fmt.Sprintf("[%c]", hotkey))
	if active {
		hotkeyBadge = mdl.styles.hotkeyActive.Render(fmt.Sprintf("[%c]", hotkey))
	}

	labelWidth := mdl.styles.menuWidth - 14 //nolint:mnd // border+pad+prefix+badge+gap
	if labelWidth < 8 {
		labelWidth = 8
	}

	padded := label
	if lipgloss.Width(padded) < labelWidth {
		padded += strings.Repeat(" ", labelWidth-lipgloss.Width(padded))
	}

	return style.Render(prefix + padded + " " + hotkeyBadge)
}

// renderHarnessList renders the harness selection rows, shared by both layout modes.
func renderHarnessList(mdl *model) string {
	var rows []string

	for idx, h := range mdl.harnesses {
		prefix := "  "
		style := mdl.styles.menuItem

		if idx == mdl.bundleInput.harnessCur {
			prefix = cursorActive
			style = mdl.styles.menuItemActive
		}

		// Dim the harness list when not focused on harness panel.
		if mdl.bundleInput.focusArea != bundleFocusHarness {
			style = lipgloss.NewStyle().Foreground(colorDim)
		}

		row := style.Render(prefix + h.name + "  " + mdl.styles.placeholder.Render(h.desc))
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderBundleResolving renders the resolving spinner screen.
func renderBundleResolving(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness", "Resolving"})

	spinnerView := mdl.bundleResolve.spinner.View()

	ref := mdl.bundleResolve.namespace + "/" + mdl.bundleResolve.slug
	if mdl.bundleResolve.version != "" {
		ref += ":" + mdl.bundleResolve.version
	}

	text := mdl.styles.spinnerText.Render(fmt.Sprintf("Resolving %s...", ref))
	body := spinnerView + " " + text

	panel := renderPanel(&mdl.styles, "Resolving", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "esc", desc: "cancel"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleConfirm renders the confirmation screen with resolved details.
func renderBundleConfirm(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness", "Confirm"})

	loadBtn := renderButton(&mdl.styles, "Load", mdl.bundleConfirm.buttonIdx == 0)
	cancelBtn := renderButton(&mdl.styles, "Cancel", mdl.bundleConfirm.buttonIdx == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, loadBtn, "  ", cancelBtn)

	lines := []string{
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleConfirm.namespace + "/" + mdl.bundleConfirm.slug),
		"",
		mdl.styles.sectionTitle.Render("Version"),
		mdl.styles.progressText.Render(mdl.bundleConfirm.version),
		"",
		mdl.styles.sectionTitle.Render("Assets"),
		mdl.styles.progressText.Render(fmt.Sprintf("%d files", mdl.bundleConfirm.assetCount)),
		"",
		mdl.styles.sectionTitle.Render("Harness"),
		mdl.styles.progressText.Render(mdl.bundleConfirm.harness),
		"",
		buttons,
	}

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Confirm", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch"},
		{key: "enter", desc: "confirm"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleProgress renders the download progress screen.
func renderBundleProgress(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness", "Downloading"})

	ref := mdl.bundleProgress.namespace + "/" + mdl.bundleProgress.slug + " v" + mdl.bundleProgress.version

	// Progress bar.
	pct := 0.0
	if mdl.bundleProgress.total > 0 {
		pct = float64(mdl.bundleProgress.current) / float64(mdl.bundleProgress.total)
	}

	bar := mdl.bundleProgress.progress.ViewAs(pct)

	label := mdl.bundleProgress.label
	if label == "" {
		label = fmt.Sprintf("%d / %d assets", mdl.bundleProgress.current, mdl.bundleProgress.total)
	}

	lines := []string{
		mdl.styles.sectionTitle.Render("Downloading"),
		mdl.styles.progressText.Render(ref),
		"",
		bar,
		"",
		mdl.styles.spinnerText.Render(label),
	}

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Progress", body, mdl.styles.menuWidth, true)

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleComplete renders the success screen.
func renderBundleComplete(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Run Harness", "Ready"})

	launchBtn := renderButton(&mdl.styles, "Launch interaction", true)

	lines := []string{
		renderStatusDot(&mdl.styles.statusOK, "Bundle ready"),
		"",
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleComplete.namespace + "/" + mdl.bundleComplete.slug + " v" + mdl.bundleComplete.version),
		"",
		mdl.styles.sectionTitle.Render("Cached at"),
		mdl.styles.placeholder.Render(mdl.bundleComplete.cachePath),
		"",
		launchBtn,
	}

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Complete", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "enter", desc: "launch"},
		{key: "esc", desc: "home"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleError renders the error screen.
func renderBundleError(mdl *model) string {
	return renderErrorScreen(mdl,
		[]string{"Home", "Run Harness", "Error"},
		mdl.bundleError.message,
		mdl.bundleError.hint,
		mdl.bundleError.buttonIdx,
	)
}
