package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderBundleInput renders the bundle reference input screen.
func renderBundleInput(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle"})

	body := renderBundleInputBody(mdl)
	panel := renderPanel(&mdl.styles, "Load bundle", body, mdl.styles.menuWidth, true)

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
// (recent + installed + 1 action link at the bottom).
func bundleListLen(mdl *model) int {
	return len(mdl.bundleInput.recentBundles) + len(mdl.bundleInput.installedBundles) + 1 // 1 action link
}

// renderBundleInputBody renders the shared panel body for the bundle input screen.
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

			label := b.ref
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

	// Action link: "Find a bundle on the Hub".
	sep := mdl.styles.placeholder.Render("── ── ── ── ── ── ──")
	findActive := mdl.bundleInput.focusArea == bundleFocusList && mdl.bundleInput.listCursor == listIdx
	findLink := renderBundleActionLink(mdl, "Find a bundle on the Hub", 'f', findActive)

	parts = append(parts, "", sep, findLink)

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

// renderBundleActionLink renders a selectable link at the bottom of the input screen.
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

// renderBundleResolving renders the resolving spinner screen.
func renderBundleResolving(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Resolving"})

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

// renderBundleProgress renders the download progress screen.
func renderBundleProgress(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Downloading"})

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

// renderBundleAction renders the Run/Install action choice screen.
func renderBundleAction(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Ready"})

	runBtn := renderButton(&mdl.styles, "Run", mdl.bundleAction.buttonIdx == 0)
	installBtn := renderButton(&mdl.styles, "Install", mdl.bundleAction.buttonIdx == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, runBtn, "  ", installBtn)

	lines := []string{
		renderStatusDot(&mdl.styles.statusOK, "Bundle ready"),
		"",
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleAction.namespace + "/" + mdl.bundleAction.slug + " v" + mdl.bundleAction.version),
		"",
		mdl.styles.sectionTitle.Render("Cached at"),
		mdl.styles.placeholder.Render(mdl.bundleAction.cachePath),
		"",
		buttons,
	}

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Action", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch"},
		{key: "enter", desc: "select"},
		{key: "esc", desc: "home"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleHarness renders the harness selection screen.
func renderBundleHarness(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Harness"})

	actionLabel := "Run"
	if mdl.bundleHarness.forInstall {
		actionLabel = "Install"
	}

	var rows []string

	for i, harnessIdx := range mdl.bundleHarness.installed {
		h := mdl.harnesses[harnessIdx]
		prefix := "  "
		style := mdl.styles.menuItem

		if i == mdl.bundleHarness.cursor {
			prefix = cursorActive
			style = mdl.styles.menuItemActive
		}

		row := style.Render(prefix + h.name + "  " + mdl.styles.placeholder.Render(h.desc))
		rows = append(rows, row)
	}

	lines := []string{
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleHarness.namespace + "/" + mdl.bundleHarness.slug + " v" + mdl.bundleHarness.version),
		"",
		mdl.styles.sectionTitle.Render("Action"),
		mdl.styles.progressText.Render(actionLabel),
		"",
		mdl.styles.sectionTitle.Render("Select harness"),
		strings.Join(rows, "\n"),
	}

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Harness", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
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

// renderBundleInstallConfirm renders the install confirmation screen.
func renderBundleInstallConfirm(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Install"})

	lines := []string{
		mdl.styles.sectionTitle.Render("Install bundle"),
		"",
		mdl.styles.progressText.Render(mdl.bundleInstallConfirm.namespace + "/" + mdl.bundleInstallConfirm.slug + " v" + mdl.bundleInstallConfirm.version),
		mdl.styles.placeholder.Render("Harness: " + mdl.bundleInstallConfirm.harness),
		mdl.styles.placeholder.Render("Target: " + mdl.bundleInstallConfirm.targetDir),
	}

	if mdl.bundleInstallConfirm.hasConflicts {
		lines = append(lines, "")

		checkbox := "[ ]"
		if mdl.bundleInstallConfirm.force {
			checkbox = "[x]"
		}

		toggleStyle := mdl.styles.menuItem
		if mdl.bundleInstallConfirm.buttonIdx == 0 {
			toggleStyle = mdl.styles.menuItemActive
		}

		lines = append(lines, toggleStyle.Render(checkbox+" Overwrite existing files"))

		conflictNote := fmt.Sprintf("%d file(s) already exist", len(mdl.bundleInstallConfirm.conflictPaths))
		lines = append(lines, mdl.styles.placeholder.Render(conflictNote))
	}

	lines = append(lines, "")

	installBtnIdx := 0
	cancelBtnIdx := 1

	if mdl.bundleInstallConfirm.hasConflicts {
		installBtnIdx = 1
		cancelBtnIdx = 2 //nolint:mnd // 3 focusable areas with toggle
	}

	installBtn := renderButton(&mdl.styles, "Install", mdl.bundleInstallConfirm.buttonIdx == installBtnIdx)
	cancelBtn := renderButton(&mdl.styles, "Cancel", mdl.bundleInstallConfirm.buttonIdx == cancelBtnIdx)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, installBtn, "  ", cancelBtn)
	lines = append(lines, buttons)

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Install", body, mdl.styles.menuWidth, true)

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

// renderBundleError renders the error screen.
func renderBundleError(mdl *model) string {
	return renderErrorScreen(mdl,
		[]string{"Home", "Load Bundle", "Error"},
		mdl.bundleError.message,
		mdl.bundleError.hint,
		mdl.bundleError.buttonIdx,
	)
}
