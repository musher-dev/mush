package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/harness"
)

// renderStatus renders the diagnostics status screen with two stacked panels.
func renderStatus(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Check Status"})

	// Diagnostics panel.
	var diagBody string

	if mdl.status.loading {
		spinnerView := mdl.status.spinner.View()
		text := mdl.styles.spinnerText.Render("Running diagnostics...")
		diagBody = spinnerView + " " + text
	} else {
		diagBody = renderStatusResults(mdl)
	}

	diagPanel := renderPanel(&mdl.styles, "Diagnostics", diagBody, mdl.styles.hubWidth, true)

	// Harnesses panel.
	var harnessBody string

	if mdl.status.harnessLoading {
		spinnerView := mdl.status.spinner.View()
		text := mdl.styles.spinnerText.Render("Checking harnesses...")
		harnessBody = spinnerView + " " + text
	} else {
		harnessBody = renderHarnessResults(mdl)
	}

	harnessPanel := renderPanel(&mdl.styles, "Harnesses", harnessBody, mdl.styles.hubWidth, true)

	// Footer hints.
	hints := []hint{
		bindingHint(mdl.keys.Back, "back"),
		bindingHint(mdl.keys.Quit, "quit"),
	}

	if !mdl.status.loading && !mdl.status.harnessLoading {
		actionHints := []hint{bindingHint(mdl.keys.Retry, "re-run")}

		if hasMissingHarnesses(mdl.status.harnessReports) {
			actionHints = append(actionHints, bindingHint(mdl.keys.Install, "install missing"))
		}

		hints = append(actionHints, hints...)
	}

	footer := renderKeyHints(&mdl.styles, hints)

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", diagPanel, "", harnessPanel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderStatusResults formats the check results into a displayable block.
func renderStatusResults(mdl *model) string {
	var lines []string

	for _, r := range mdl.status.results {
		symbol := statusSymbolStyled(&mdl.styles, r.Status)
		line := symbol + " " + mdl.styles.sectionTitle.Render(r.Name) + "  " + mdl.styles.progressText.Render(r.Message)
		lines = append(lines, line)

		if r.Detail != "" {
			detail := "    " + mdl.styles.placeholder.Render(r.Detail)
			lines = append(lines, detail)
		}
	}

	// Summary line.
	lines = append(lines, "")

	var parts []string

	if mdl.status.passed > 0 {
		parts = append(parts, mdl.styles.statusOK.Render(fmt.Sprintf("%d passed", mdl.status.passed)))
	}

	if mdl.status.failed > 0 {
		parts = append(parts, mdl.styles.statusError.Render(fmt.Sprintf("%d failed", mdl.status.failed)))
	}

	if mdl.status.warnings > 0 {
		parts = append(parts, mdl.styles.statusWarning.Render(fmt.Sprintf("%d warnings", mdl.status.warnings)))
	}

	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, "  "))
	}

	return strings.Join(lines, "\n")
}

// renderHarnessResults formats harness health reports into a displayable block.
func renderHarnessResults(mdl *model) string {
	if len(mdl.status.harnessReports) == 0 {
		return mdl.styles.placeholder.Render("No harnesses registered")
	}

	var sections []string

	for _, report := range mdl.status.harnessReports {
		var lines []string

		lines = append(lines, mdl.styles.sectionTitle.Render(report.DisplayName))

		for _, r := range report.Results {
			symbol := harnessStatusSymbol(&mdl.styles, r.Status)
			line := "  " + symbol + " " + mdl.styles.sectionTitle.Render(r.Check) + "  " + mdl.styles.progressText.Render(r.Message)
			lines = append(lines, line)

			if r.Detail != "" {
				detail := "    " + mdl.styles.placeholder.Render(r.Detail)
				lines = append(lines, detail)
			}
		}

		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// statusSymbolStyled returns a styled check symbol for the given doctor status.
func statusSymbolStyled(styles *theme, status doctor.Status) string {
	switch status {
	case doctor.StatusPass:
		return styles.statusOK.Render(status.Symbol())
	case doctor.StatusWarn:
		return styles.statusWarning.Render(status.Symbol())
	case doctor.StatusFail:
		return styles.statusError.Render(status.Symbol())
	default:
		return status.Symbol()
	}
}

// harnessStatusSymbol returns a styled check symbol for the given health status.
func harnessStatusSymbol(styles *theme, status harness.HealthStatus) string {
	switch status {
	case harness.HealthPass:
		return styles.statusOK.Render("\u2713")
	case harness.HealthWarn:
		return styles.statusWarning.Render("\u26A0")
	case harness.HealthFail:
		return styles.statusError.Render("\u2717")
	default:
		return "?"
	}
}

// hasMissingHarnesses returns true if any harness report has a failed binary check.
func hasMissingHarnesses(reports []*harness.HealthReport) bool {
	for _, r := range reports {
		if len(r.Results) > 0 && r.Results[0].Status == harness.HealthFail {
			return true
		}
	}

	return false
}
