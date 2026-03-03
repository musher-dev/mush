package nav

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderWorkerHabitats renders the habitat selection screen.
func renderWorkerHabitats(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Start Worker", "Habitat"})

	var body string

	if mdl.workerHabitats.loading {
		spinnerView := mdl.workerHabitats.spinner.View()
		text := mdl.styles.spinnerText.Render("Loading habitats...")
		body = spinnerView + " " + text
	} else {
		var rows []string

		for idx, h := range mdl.workerHabitats.habitats {
			prefix := cursorBlank
			style := mdl.styles.menuItem

			if idx == mdl.workerHabitats.cursor {
				prefix = cursorActive
				style = mdl.styles.menuItemActive
			}

			row := style.Render(prefix + h.Name + "  " + mdl.styles.placeholder.Render(h.Slug))
			rows = append(rows, row)
		}

		body = strings.Join(rows, "\n")
	}

	panel := renderPanel(&mdl.styles, "Select Habitat", body, mdl.styles.menuWidth, true)

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

// renderWorkerQueues renders the queue selection screen.
func renderWorkerQueues(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Start Worker", "Queue"})

	ctxLine := mdl.styles.sectionTitle.Render("Habitat") + "  " +
		mdl.styles.progressText.Render(mdl.workerQueues.habitatName)

	var body string

	if mdl.workerQueues.loading {
		spinnerView := mdl.workerQueues.spinner.View()
		text := mdl.styles.spinnerText.Render("Loading queues...")
		body = ctxLine + "\n\n" + spinnerView + " " + text
	} else {
		var rows []string

		for idx, queue := range mdl.workerQueues.queues {
			prefix := cursorBlank
			style := mdl.styles.menuItem

			if idx == mdl.workerQueues.cursor {
				prefix = cursorActive
				style = mdl.styles.menuItemActive
			}

			row := style.Render(prefix + queue.Name + "  " + mdl.styles.placeholder.Render(queue.Slug))
			rows = append(rows, row)
		}

		body = ctxLine + "\n\n" + strings.Join(rows, "\n")
	}

	panel := renderPanel(&mdl.styles, "Select Queue", body, mdl.styles.menuWidth, true)

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

// renderWorkerHarness renders the harness selection screen.
func renderWorkerHarness(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Start Worker", "Harness"})

	ctxLines := mdl.styles.sectionTitle.Render("Habitat") + "  " +
		mdl.styles.progressText.Render(mdl.workerHarness.habitatName) + "\n" +
		mdl.styles.sectionTitle.Render("Queue") + "  " +
		mdl.styles.progressText.Render(mdl.workerHarness.queueName)

	var rows []string

	for idx, h := range mdl.harnesses {
		prefix := cursorBlank
		style := mdl.styles.menuItem

		if idx == mdl.workerHarness.cursor {
			prefix = cursorActive
			style = mdl.styles.menuItemActive
		}

		row := style.Render(prefix + h.name + "  " + mdl.styles.placeholder.Render(h.desc))
		rows = append(rows, row)
	}

	body := ctxLines + "\n\n" + strings.Join(rows, "\n")
	panel := renderPanel(&mdl.styles, "Select Harness", body, mdl.styles.menuWidth, true)

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

// renderWorkerChecking renders the instruction checking spinner screen.
func renderWorkerChecking(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Start Worker", "Checking"})

	spinnerView := mdl.workerChecking.spinner.View()
	text := mdl.styles.spinnerText.Render(
		fmt.Sprintf("Checking instructions for %s...", mdl.workerChecking.queueName),
	)

	body := spinnerView + " " + text
	panel := renderPanel(&mdl.styles, "Checking", body, mdl.styles.menuWidth, true)

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

// renderWorkerConfirm renders the confirmation screen.
func renderWorkerConfirm(mdl *model) string {
	crumbs := renderBreadcrumb(&mdl.styles, []string{"Home", "Start Worker", "Confirm"})

	startBtn := renderButton(&mdl.styles, "Start", mdl.workerConfirm.buttonIdx == 0)
	cancelBtn := renderButton(&mdl.styles, "Cancel", mdl.workerConfirm.buttonIdx == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, startBtn, "  ", cancelBtn)

	lines := []string{
		mdl.styles.sectionTitle.Render("Habitat"),
		mdl.styles.progressText.Render(mdl.workerConfirm.habitatName),
		"",
		mdl.styles.sectionTitle.Render("Queue"),
		mdl.styles.progressText.Render(mdl.workerConfirm.queueName),
		"",
		mdl.styles.sectionTitle.Render("Harness"),
		mdl.styles.progressText.Render(mdl.workerConfirm.harness),
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

// renderWorkerError renders the worker error screen.
func renderWorkerError(mdl *model) string {
	return renderErrorScreen(mdl,
		[]string{"Home", "Start Worker", "Error"},
		mdl.workerError.message,
		mdl.workerError.hint,
	)
}
