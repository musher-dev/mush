package nav

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/musher-dev/mush/internal/transcript"
)

// renderHistoryList renders the transcript session list screen.
func renderHistoryList(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "View History"})

	var body string

	switch {
	case mdl.history.loading:
		spinnerView := mdl.history.spinner.View()
		text := mdl.styles.spinnerText.Render("Loading sessions...")
		body = spinnerView + " " + text

	case mdl.history.errorMsg != "":
		body = renderStatusDot(&mdl.styles.statusError, mdl.history.errorMsg)

	case len(mdl.history.sessions) == 0:
		body = mdl.styles.placeholder.Render("No transcript sessions found")

	default:
		var rows []string

		for idx, s := range mdl.history.sessions {
			rows = append(rows, renderHistoryRow(mdl, idx, s))
		}

		body = strings.Join(rows, "\n")

		// Summary line.
		open := 0

		for _, s := range mdl.history.sessions {
			if s.ClosedAt == nil {
				open++
			}
		}

		summary := fmt.Sprintf("%d sessions", len(mdl.history.sessions))
		if open > 0 {
			summary += fmt.Sprintf(", %d open", open)
		}

		body += "\n\n" + mdl.styles.placeholder.Render(summary)
	}

	panel := renderPanel(&mdl.styles, "Sessions", body, mdl.styles.hubWidth, true)

	hints := []hint{
		bindingHint(mdl.keys.Back, "back"),
		bindingHint(mdl.keys.Quit, "quit"),
	}

	if !mdl.history.loading {
		actionHints := []hint{
			bindingHint(mdl.keys.Retry, "refresh"),
			navigationHint(mdl.keys.Up, mdl.keys.Down, "navigate"),
			bindingHint(mdl.keys.Select, "view"),
		}

		hints = append(actionHints, hints...)
	}

	footer := renderKeyHints(&mdl.styles, hints)

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHistoryRow renders a single session row in the history list.
func renderHistoryRow(mdl *model, idx int, s transcript.Session) string {
	prefix := cursorBlank
	if idx == mdl.history.cursor {
		prefix = cursorActive
	}

	id := s.SessionID
	if len(id) > historyIDWidth {
		id = id[:historyIDWidth]
	}

	// Date: "Jan 02, 15:04"
	dateStr := s.StartedAt.Local().Format("Jan 02, 15:04")

	// Duration or status indicator.
	var durStr string
	if s.ClosedAt != nil {
		durStr = formatDuration(s.ClosedAt.Sub(s.StartedAt))
	} else {
		durStr = "running"
	}

	// Status dot: green for closed, yellow for open.
	var statusDot string
	if s.ClosedAt != nil {
		statusDot = mdl.styles.statusOK.Render("\u25CF")
	} else {
		statusDot = mdl.styles.statusWarning.Render("\u25CF")
	}

	row := prefix +
		mdl.styles.progressText.Render(id) + "  " +
		mdl.styles.placeholder.Render(dateStr) + "  " +
		statusDot + " " +
		mdl.styles.placeholder.Render(durStr)

	return mdl.styles.menuItem.Render(row)
}

// renderHistoryDetail renders the transcript session detail screen.
func renderHistoryDetail(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "View History", "Detail"})

	var body string

	switch {
	case mdl.historyDetail.loading:
		spinnerView := mdl.historyDetail.spinner.View()
		text := mdl.styles.spinnerText.Render("Loading events...")
		body = spinnerView + " " + text

	case mdl.historyDetail.errorMsg != "":
		body = renderStatusDot(&mdl.styles.statusError, mdl.historyDetail.errorMsg)

	case len(mdl.historyDetail.lines) == 0:
		body = renderHistoryDetailHeader(mdl) + "\n\n" +
			mdl.styles.placeholder.Render("No events recorded")

	default:
		header := renderHistoryDetailHeader(mdl)

		// Calculate visible window.
		chrome := historyChromeLines
		visible := mdl.height - chrome

		if visible < 1 {
			visible = 1
		}

		start := mdl.historyDetail.scrollOffset
		end := start + visible

		if end > len(mdl.historyDetail.lines) {
			end = len(mdl.historyDetail.lines)
		}

		if start > end {
			start = end
		}

		visibleLines := mdl.historyDetail.lines[start:end]
		scrollInfo := mdl.styles.placeholder.Render(
			fmt.Sprintf("Lines %d-%d of %d", start+1, end, len(mdl.historyDetail.lines)),
		)

		body = header + "\n\n" + strings.Join(visibleLines, "\n") + "\n\n" + scrollInfo
	}

	panel := renderPanel(&mdl.styles, "Transcript", body, mdl.styles.hubWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		navigationHint(mdl.keys.Up, mdl.keys.Down, "scroll"),
		bindingHint(mdl.keys.Back, "back"),
		bindingHint(mdl.keys.Quit, "quit"),
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHistoryDetailHeader renders the session metadata header.
func renderHistoryDetailHeader(mdl *model) string {
	s := mdl.historyDetail.session

	id := s.SessionID
	if len(id) > historyIDWidth {
		id = id[:historyIDWidth]
	}

	dateStr := s.StartedAt.Local().Format("Jan 02, 15:04:05")

	var statusPart string

	if s.ClosedAt != nil {
		durStr := formatDuration(s.ClosedAt.Sub(s.StartedAt))
		statusPart = mdl.styles.statusOK.Render("\u25CF") + " " +
			mdl.styles.placeholder.Render("closed") + "  " +
			mdl.styles.placeholder.Render(durStr)
	} else {
		statusPart = mdl.styles.statusWarning.Render("\u25CF") + " " +
			mdl.styles.placeholder.Render("running")
	}

	eventCount := len(mdl.historyDetail.events)
	lineCount := len(mdl.historyDetail.lines)
	stats := fmt.Sprintf("%d events, %d lines", eventCount, lineCount)

	return mdl.styles.sectionTitle.Render(id) + "  " +
		mdl.styles.placeholder.Render(dateStr) + "  " +
		statusPart + "\n" +
		mdl.styles.placeholder.Render(stats)
}

// formatDuration renders a duration as a compact human-friendly string.
func formatDuration(dur time.Duration) string {
	switch {
	case dur < time.Second:
		return "<1s"
	case dur < time.Minute:
		return fmt.Sprintf("%ds", int(dur.Seconds()))
	case dur < time.Hour:
		mins := int(dur.Minutes())
		secs := int(dur.Seconds()) % secondsPerMinute

		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}

		return fmt.Sprintf("%dm %ds", mins, secs)
	default:
		hours := int(dur.Hours())
		mins := int(dur.Minutes()) % secondsPerMinute

		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}

		return fmt.Sprintf("%dh %dm", hours, mins)
	}
}
