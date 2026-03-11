package nav

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/musher-dev/mush/internal/client"
)

// renderBundleInput renders the bundle reference input screen.
func renderBundleInput(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle"})

	leftActive := mdl.bundleInput.focusArea != bundleFocusMyBundles
	body := renderBundleInputBody(mdl)
	leftPanel := renderPanel(&mdl.styles, "Load bundle", body, mdl.styles.menuWidth, leftActive)

	var panels string

	if mdl.styles.layout == layoutTwoPanel {
		rightActive := mdl.bundleInput.focusArea == bundleFocusMyBundles
		rightBody := renderMyBundlesBody(mdl)
		rightPanel := renderPanel(&mdl.styles, "My Bundles", rightBody, mdl.styles.menuWidth, rightActive)
		panels = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
	} else {
		panels = leftPanel
	}

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch focus"},
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

	contentsSection := renderBundleContents(&mdl.styles, mdl.bundleAction.layers, mdl.styles.layout, mdl.styles.menuWidth)

	lines := []string{
		renderStatusDot(&mdl.styles.statusOK, "Bundle ready"),
		"",
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleAction.namespace + "/" + mdl.bundleAction.slug + " v" + mdl.bundleAction.version),
		"",
		contentsSection,
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

// assetGroup holds aggregated info for one asset type.
type assetGroup struct {
	label string
	order int
	count int
	size  int64
	paths []string
}

// assetTypeInfo maps asset type strings to display labels and sort order.
var assetTypeInfo = map[string]struct {
	label string
	order int
}{
	"skill":            {label: "Skills", order: 1},
	"agent_definition": {label: "Agents", order: 2},
	"tool_config":      {label: "Tools", order: 3},
}

// groupLayers groups bundle layers by asset type, sorted by fixed order.
func groupLayers(layers []client.BundleLayer) []assetGroup {
	grouped := make(map[string]*assetGroup)

	for _, layer := range layers {
		info, ok := assetTypeInfo[layer.AssetType]
		if !ok {
			info = struct {
				label string
				order int
			}{label: layer.AssetType, order: 99} //nolint:mnd // unknown types sort last
		}

		group, exists := grouped[layer.AssetType]
		if !exists {
			group = &assetGroup{label: info.label, order: info.order}
			grouped[layer.AssetType] = group
		}

		group.count++
		group.size += layer.SizeBytes
		group.paths = append(group.paths, layer.LogicalPath)
	}

	result := make([]assetGroup, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, *group)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].order != result[j].order {
			return result[i].order < result[j].order
		}

		return result[i].label < result[j].label
	})

	return result
}

// formatBytes returns a human-readable size string.
func formatBytes(n int64) string {
	const (
		kiloByte = 1024
		megaByte = 1024 * 1024
	)

	switch {
	case n >= megaByte:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(megaByte))
	case n >= kiloByte:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kiloByte))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// maxPathsShown controls path truncation: when a group has more paths than this,
// only maxPathsShown-1 paths are shown followed by a "+N more" line.
const maxPathsShown = 3

// renderBundleContents renders the Contents section for the bundle action screen.
func renderBundleContents(styles *theme, layers []client.BundleLayer, layout layoutMode, availWidth int) string {
	title := styles.sectionTitle.Render("Contents")

	if len(layers) == 0 {
		return title + "\n" + styles.placeholder.Render("No assets")
	}

	groups := groupLayers(layers)
	lines := []string{title}

	// Available width for content inside the panel (minus border/padding).
	contentWidth := availWidth - 8 //nolint:mnd // border + padding
	if contentWidth < 20 {         //nolint:mnd // minimum usable width
		contentWidth = 20
	}

	for _, group := range groups {
		// Group header: "Skills (2)                   1.8 KB"
		header := fmt.Sprintf("%s (%d)", group.label, group.count)
		sizeStr := formatBytes(group.size)

		pad := contentWidth - lipgloss.Width(header) - lipgloss.Width(sizeStr)
		if pad < 1 {
			pad = 1
		}

		headerLine := styles.sectionTitle.Render(header) + strings.Repeat(" ", pad) + styles.placeholder.Render(sizeStr)
		lines = append(lines, headerLine)

		// File paths (skip in compact/minimal layouts).
		if layout >= layoutSingle {
			showCount := len(group.paths)
			truncated := false

			if showCount > maxPathsShown {
				showCount = maxPathsShown - 1 // show N-1 + "more" line
				truncated = true
			}

			for _, path := range group.paths[:showCount] {
				lines = append(lines, styles.placeholder.Render("  "+path))
			}

			if truncated {
				remaining := len(group.paths) - showCount
				lines = append(lines, styles.placeholder.Render(fmt.Sprintf("  +%d more", remaining)))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// renderMyBundlesBody renders the body content for the "My Bundles" panel.
func renderMyBundlesBody(mdl *model) string {
	if mdl.myBundles.loading {
		return mdl.myBundles.spinner.View() + " " + mdl.styles.spinnerText.Render("Loading...")
	}

	if mdl.myBundles.errorMsg != "" {
		return mdl.styles.placeholder.Render(mdl.myBundles.errorMsg)
	}

	if len(mdl.myBundles.bundles) == 0 {
		return mdl.styles.placeholder.Render("No bundles published yet")
	}

	var rows []string

	for i := range mdl.myBundles.bundles {
		b := &mdl.myBundles.bundles[i]
		active := mdl.bundleInput.focusArea == bundleFocusMyBundles && mdl.myBundles.cursor == i

		label := b.Publisher.Handle + "/" + b.Slug
		if b.LatestVersion != "" {
			label += " v" + b.LatestVersion
		}

		detail := formatTimeAgo(b.UpdatedAt)
		row := renderBundleListRow(mdl, label, detail, active)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}
