package nav

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/musher-dev/mush/internal/harness"
)

// harnessCollapseIndicator is the prefix for a collapsed harness row.
const harnessCollapseIndicator = "\u25B6 " // ▶

// harnessExpandIndicator is the prefix for an expanded harness row.
const harnessExpandIndicator = "\u25BC " // ▼

// renderHarnessPanel renders the harness sidebar panel for the home screen.
func renderHarnessPanel(mdl *model, width int, active bool) string {
	content := renderHarnessPanelContent(mdl, width)

	return renderPanel(&mdl.styles, "Harnesses", content, width, active)
}

// renderHarnessPanelContent renders the inner content of the harness panel.
func renderHarnessPanelContent(mdl *model, panelWidth int) string {
	if mdl.homeHarness.loading {
		return mdl.styles.placeholder.Render("Detecting harnesses...")
	}

	if len(mdl.homeHarness.statuses) == 0 {
		return mdl.styles.placeholder.Render("No harnesses registered")
	}

	// Content width = panel width - border(2) - padding(4).
	contentWidth := panelWidth - 6 //nolint:mnd // border + padding
	if contentWidth < 10 {
		contentWidth = 10
	}

	var rows []string

	for idx, status := range mdl.homeHarness.statuses {
		isSelected := mdl.homeFocusArea == 1 && idx == mdl.homeHarness.cursor
		isExpanded := idx == mdl.homeHarness.expanded

		row := renderHarnessRow(mdl, status, contentWidth, isSelected, isExpanded)
		rows = append(rows, row)

		if isExpanded {
			rows = append(rows, renderHarnessExpandedDetail(mdl, status, contentWidth))
		}
	}

	return strings.Join(rows, "\n")
}

// renderHarnessRow renders a single collapsed or expandable harness row.
func renderHarnessRow(mdl *model, status harnessQuickStatus, contentWidth int, selected, expanded bool) string {
	indicator := harnessCollapseIndicator

	if expanded {
		indicator = harnessExpandIndicator
	}

	// Right-aligned status text.
	var statusText string

	if status.installed {
		ver := status.version
		if ver != "" && !strings.HasPrefix(ver, "v") {
			ver = "v" + ver
		}

		statusText = mdl.styles.statusOK.Render(ver)
	} else {
		statusText = mdl.styles.placeholder.Render("not installed")
	}

	statusVisualWidth := ansi.StringWidth(statusText)

	// Calculate label width: content - indicator(2) - status - gap(1).
	labelWidth := contentWidth - 2 - statusVisualWidth - 1 //nolint:mnd // indicator + gap
	if labelWidth < 4 {
		labelWidth = 4
	}

	label := status.displayName
	labelVisual := ansi.StringWidth(label)

	if labelVisual < labelWidth {
		label += strings.Repeat(" ", labelWidth-labelVisual)
	}

	line := indicator + label + " " + statusText

	if selected {
		return mdl.styles.menuItemActive.Render(line)
	}

	return mdl.styles.menuItem.Render(line)
}

// renderHarnessExpandedDetail renders the expanded health detail for a harness.
func renderHarnessExpandedDetail(mdl *model, status harnessQuickStatus, contentWidth int) string {
	indent := "  "

	if mdl.harnessExpand.loading {
		spinnerView := mdl.harnessExpand.spinner.View()
		text := mdl.styles.spinnerText.Render("Checking...")

		return indent + spinnerView + " " + text
	}

	report := mdl.harnessExpand.report
	if report == nil {
		return indent + mdl.styles.placeholder.Render("No data")
	}

	// Available width for message text after: indent(2) + symbol(2) + check(~12) + gap(2).
	checkColWidth := 12

	msgMaxWidth := contentWidth - 2 - 2 - checkColWidth - 2 //nolint:mnd // indent + symbol + check col + gap
	if msgMaxWidth < 10 {
		msgMaxWidth = 10
	}

	var lines []string

	for _, r := range report.Results {
		symbol := harnessStatusSymbol(&mdl.styles, r.Status)
		checkLabel := padRight(r.Check, checkColWidth)

		msg := cleanHealthMessage(r.Check, r.Message)
		if ansi.StringWidth(msg) > msgMaxWidth {
			msg = msg[:msgMaxWidth-1] + "…"
		}

		line := indent + symbol + " " + mdl.styles.sectionTitle.Render(checkLabel) + mdl.styles.progressText.Render(msg)
		lines = append(lines, line)
	}

	// If not installed, show install hint.
	if !status.installed && report.InstallHint != "" {
		lines = append(lines,
			"",
			indent+mdl.styles.placeholder.Render(report.InstallHint),
			indent+mdl.styles.hintKey.Render("Press i to install"),
		)
	}

	return strings.Join(lines, "\n")
}

// cleanHealthMessage strips verbose prefixes from health check messages
// to show only the essential value (path, version number, etc.).
func cleanHealthMessage(check, message string) string {
	switch check {
	case "Binary":
		// "claude found at /usr/local/bin/claude" → "/usr/local/bin/claude"
		// "claude not found in PATH" → kept as-is (failure case)
		if idx := strings.Index(message, " found at "); idx >= 0 {
			return message[idx+len(" found at "):]
		}

	case "Version":
		// "2.1.68 (Claude Code)" → "2.1.68"
		// Strip parenthetical suffix.
		if idx := strings.Index(message, " ("); idx >= 0 {
			return message[:idx]
		}
	}

	return message
}

// padRight pads a string with spaces to the given width.
func padRight(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}

	return s + strings.Repeat(" ", width-w)
}

// harnessInstallCommands returns install commands for a specific harness by name.
func harnessInstallCommands(name string) [][]string {
	spec, ok := harness.GetProvider(name)
	if !ok || spec.Status == nil || len(spec.Status.InstallCommand) == 0 {
		return nil
	}

	return [][]string{spec.Status.InstallCommand}
}

// harnessPanelWidth computes the width of the harness panel for two-panel mode.
func harnessPanelWidth(totalWidth, menuWidth int) int {
	// Gap of 2 between menu and harness panel.
	available := totalWidth - menuWidth - 6 //nolint:mnd // gap(2) + centering margin(4)
	if available < 30 {
		return 30
	}

	if available > 52 {
		return 52
	}

	return available
}

// harnessQuickStatusVersion returns a cleaned version string for display.
// Handles outputs like "Claude Code 2.1.68 (Claude Code)" → "2.1.68"
// and "codex-cli 0.106.0" → "0.106.0".
func harnessQuickStatusVersion(raw string) string {
	// Strip common prefixes from version output.
	v := strings.TrimSpace(raw)

	// Remove parenthetical suffixes like "(Claude Code)".
	if idx := strings.Index(v, "("); idx > 0 {
		v = strings.TrimSpace(v[:idx])
	}

	// Extract the last space-separated token if it looks like a version.
	if idx := strings.LastIndex(v, " "); idx >= 0 {
		candidate := v[idx+1:]
		if candidate != "" && (candidate[0] == 'v' || (candidate[0] >= '0' && candidate[0] <= '9')) {
			return candidate
		}
	}

	return v
}

func formatHarnessPanelWidth(mdl *model) int {
	return harnessPanelWidth(mdl.width, mdl.styles.menuWidth)
}
