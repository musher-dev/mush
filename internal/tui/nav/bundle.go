package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderBundleInput renders the bundle slug input screen.
func renderBundleInput(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle"})

	title := mdl.styles.sectionTitle.Render("Bundle slug")
	input := mdl.bundleInput.textInput.View()

	// Harness selector.
	harnessTitle := mdl.styles.sectionTitle.Render("Harness")

	var harnessRows []string

	for idx, h := range mdl.harnesses {
		prefix := "  "
		style := mdl.styles.menuItem

		if idx == mdl.bundleInput.harnessCur {
			prefix = cursorActive
			style = mdl.styles.menuItemActive
		}

		// Dim the harness list when input is focused.
		if mdl.bundleInput.focusOnInput {
			style = lipgloss.NewStyle().Foreground(colorDim)
		}

		row := style.Render(prefix + h.name + "  " + mdl.styles.placeholder.Render(h.desc))
		harnessRows = append(harnessRows, row)
	}

	harnessList := strings.Join(harnessRows, "\n")

	focusHint := mdl.styles.placeholder.Render("tab to switch focus")

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		input,
		"",
		harnessTitle,
		harnessList,
		"",
		focusHint,
	)

	panel := renderPanel(&mdl.styles, "Load Bundle", body, mdl.styles.menuWidth, true)

	hubHint := mdl.styles.placeholder.Render("Don't know the slug? Press ") +
		mdl.styles.hintKey.Render("e") +
		mdl.styles.placeholder.Render(" to explore the hub")

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "tab", desc: "switch focus"},
		{key: "enter", desc: "resolve"},
		{key: "e", desc: "explore hub"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", hubHint, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderBundleResolving renders the resolving spinner screen.
func renderBundleResolving(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Resolving"})

	spinnerView := mdl.bundleResolve.spinner.View()

	ref := mdl.bundleResolve.slug
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
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Confirm"})

	loadBtn := renderButton(&mdl.styles, "Load", mdl.bundleConfirm.buttonIdx == 0)
	cancelBtn := renderButton(&mdl.styles, "Cancel", mdl.bundleConfirm.buttonIdx == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, loadBtn, "  ", cancelBtn)

	lines := []string{
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleConfirm.slug),
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
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Downloading"})

	ref := mdl.bundleProgress.slug + " v" + mdl.bundleProgress.version

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
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Load Bundle", "Ready"})

	launchBtn := renderButton(&mdl.styles, "Launch session", true)

	lines := []string{
		renderStatusDot(&mdl.styles.statusOK, "Bundle ready"),
		"",
		mdl.styles.sectionTitle.Render("Bundle"),
		mdl.styles.progressText.Render(mdl.bundleComplete.slug + " v" + mdl.bundleComplete.version),
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
		[]string{"Home", "Load Bundle", "Error"},
		mdl.bundleError.message,
		mdl.bundleError.hint,
	)
}
