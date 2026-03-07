package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderHubExplore renders the hub exploration screen.
func renderHubExplore(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Find a Bundle"})

	// Search input.
	searchLabel := mdl.styles.hintKey.Render("/") + " "
	searchView := searchLabel + mdl.hubExplore.searchInput.View()

	// Results area.
	var resultsView string

	switch {
	case mdl.hubExplore.loading:
		resultsView = mdl.hubExplore.spinner.View() + " " + mdl.styles.spinnerText.Render("Searching...")
	case mdl.hubExplore.errorMsg != "":
		resultsView = mdl.styles.statusError.Render("Error: " + mdl.hubExplore.errorMsg)
	case len(mdl.hubExplore.results) == 0:
		resultsView = mdl.styles.placeholder.Render("No bundles found")
	default:
		resultsView = renderHubResultsList(mdl)
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		searchView,
		"",
		resultsView,
	)

	panel := renderPanel(&mdl.styles, "Find a Bundle", body, mdl.styles.hubWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "/", desc: "search"},
		{key: "j/k", desc: "navigate"},
		{key: "enter", desc: "view"},
		{key: "i", desc: "install"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbs, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHubResultsList renders the bundle results list.
func renderHubResultsList(mdl *model) string {
	results := mdl.hubExplore.results

	maxVisible := 5 //nolint:mnd // max visible items
	startIdx := 0

	if mdl.hubExplore.resultCur >= maxVisible {
		startIdx = mdl.hubExplore.resultCur - maxVisible + 1
	}

	endIdx := startIdx + maxVisible
	if endIdx > len(results) {
		endIdx = len(results)
	}

	var rows []string

	for idx := startIdx; idx < endIdx; idx++ {
		rows = append(rows, renderHubResultItem(mdl, idx))
	}

	list := strings.Join(rows, "\n")

	if mdl.hubExplore.hasMore {
		more := mdl.styles.placeholder.Render("  Press l to load more...")
		list += "\n" + more
	}

	return list
}

// renderHubResultItem renders a single hub bundle result.
func renderHubResultItem(mdl *model, idx int) string {
	b := mdl.hubExplore.results[idx]
	active := idx == mdl.hubExplore.resultCur

	prefix := cursorBlank
	if active {
		prefix = cursorActive
	}

	name := b.DisplayName
	if name == "" {
		name = b.Slug
	}

	ref := b.Publisher.Handle + "/" + b.Slug

	nameStyle := mdl.styles.progressText
	if active {
		nameStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	}

	trustMark := ""
	if b.Publisher.TrustTier == "verified" {
		trustMark = " " + mdl.styles.hubTrustVerified.Render("\u2713")
	}

	line1 := prefix + nameStyle.Render(name) + "  " + mdl.styles.placeholder.Render(ref) + trustMark

	// Summary line.
	summary := b.Summary

	maxSumLen := mdl.styles.hubWidth - 12 //nolint:mnd // padding + indent
	if maxSumLen > 0 && len(summary) > maxSumLen {
		summary = summary[:maxSumLen-3] + "..." //nolint:mnd // ellipsis
	}

	line2 := "    " + mdl.styles.placeholder.Render(summary)

	// Stats line.
	stars := fmt.Sprintf("\u2605 %d", b.StarsCount)
	downloads := fmt.Sprintf("\u2193 %s", formatCount(b.DownloadsTotal))
	ver := "v" + b.LatestVersion
	line3 := "    " + mdl.styles.hubStats.Render(stars+"  "+downloads+"  "+ver)

	return line1 + "\n" + line2 + "\n" + line3
}

// renderHubDetail renders the hub bundle detail screen.
func renderHubDetail(mdl *model) string {
	crumbs := []string{"Home", "Find a Bundle"}

	if mdl.hubDetail.detail != nil {
		name := mdl.hubDetail.detail.DisplayName
		if name == "" {
			name = mdl.hubDetail.detail.Slug
		}

		crumbs = append(crumbs, name)
	} else {
		crumbs = append(crumbs, mdl.hubDetail.slug)
	}

	crumbLine := renderBreadcrumb(&mdl.styles, crumbs)

	var body string

	switch {
	case mdl.hubDetail.loading:
		body = mdl.hubDetail.spinner.View() + " " + mdl.styles.spinnerText.Render("Loading...")
	case mdl.hubDetail.errorMsg != "":
		body = mdl.styles.statusError.Render("Error: " + mdl.hubDetail.errorMsg)
	case mdl.hubDetail.detail != nil:
		body = renderHubDetailContent(mdl)
	default:
		body = mdl.styles.placeholder.Render("No details available")
	}

	panel := renderPanel(&mdl.styles, "Bundle Detail", body, mdl.styles.hubWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		{key: "i", desc: "install"},
		{key: "j/k", desc: "scroll"},
		{key: "esc", desc: "back"},
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbLine, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderHubDetailContent renders the bundle detail body.
func renderHubDetailContent(mdl *model) string {
	detail := mdl.hubDetail.detail

	name := detail.DisplayName
	if name == "" {
		name = detail.Slug
	}

	ref := detail.Publisher.Handle + "/" + detail.Slug

	var trustBadge string

	switch detail.Publisher.TrustTier {
	case "verified":
		trustBadge = "  " + mdl.styles.hubTrustVerified.Render("\u2713 verified")
	case "community":
		trustBadge = "  " + mdl.styles.hubTrustCommunity.Render("community")
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(name),
		mdl.styles.placeholder.Render(ref) + trustBadge,
		"",
	}

	metaRows := []struct{ label, value string }{
		{"Version", detail.LatestVersion},
		{"License", detail.License},
		{"Stars", fmt.Sprintf("%d", detail.StarsCount)},
		{"Downloads", formatCount(detail.DownloadsTotal)},
	}

	for _, row := range metaRows {
		if row.value == "" || row.value == "0" {
			continue
		}

		lines = append(lines,
			mdl.styles.sectionTitle.Render(fmt.Sprintf("%-10s", row.label))+
				mdl.styles.progressText.Render(row.value),
		)
	}

	desc := detail.Description
	if desc == "" {
		desc = detail.Summary
	}

	if desc != "" {
		lines = append(lines, "", mdl.styles.progressText.Render(desc))
	}

	lines = append(lines, "", renderButton(&mdl.styles, "Install Bundle", true))

	// Apply scroll offset.
	if mdl.hubDetail.scrollOffset > 0 {
		off := mdl.hubDetail.scrollOffset
		if off > len(lines) {
			off = len(lines)
		}

		lines = lines[off:]
	}

	return strings.Join(lines, "\n")
}

// formatCount formats a number with K/M suffixes.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
