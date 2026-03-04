package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleHarnessPanelKey processes key events when the harness panel is focused on the home screen.
func (m *model) handleHarnessPanelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.homeHarness.cursor < len(m.homeHarness.statuses)-1 {
			m.homeHarness.cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.homeHarness.cursor > 0 {
			m.homeHarness.cursor--
		}

	case key.Matches(msg, m.keys.Select):
		return m.toggleHarnessExpansion()

	case key.Matches(msg, m.keys.Back):
		if m.homeHarness.expanded >= 0 {
			m.homeHarness.expanded = -1
			m.harnessExpand.loading = false
			m.harnessExpand.report = nil

			return m, nil
		}

		// Esc with nothing expanded: switch focus back to menu.
		m.homeFocusArea = 0

	case key.Matches(msg, m.keys.Install):
		return m.handleHarnessInstall()
	}

	return m, nil
}

// toggleHarnessExpansion toggles the inline expansion of the selected harness.
func (m *model) toggleHarnessExpansion() (tea.Model, tea.Cmd) {
	if len(m.homeHarness.statuses) == 0 {
		return m, nil
	}

	cur := m.homeHarness.cursor

	// If already expanded, collapse.
	if m.homeHarness.expanded == cur {
		m.homeHarness.expanded = -1
		m.harnessExpand.loading = false
		m.harnessExpand.report = nil

		return m, nil
	}

	// Expand and run health check.
	m.homeHarness.expanded = cur
	m.harnessExpand.loading = true
	m.harnessExpand.report = nil

	name := m.homeHarness.statuses[cur].name

	return m, tea.Batch(m.harnessExpand.spinner.Tick, cmdRunSingleHarnessHealthCheck(name))
}

// handleHarnessInstall triggers install for the expanded not-installed harness.
func (m *model) handleHarnessInstall() (tea.Model, tea.Cmd) {
	if m.homeHarness.expanded < 0 || m.homeHarness.expanded >= len(m.homeHarness.statuses) {
		return m, nil
	}

	qs := m.homeHarness.statuses[m.homeHarness.expanded]
	if qs.installed {
		return m, nil
	}

	cmds := harnessInstallCommands(qs.name)
	if len(cmds) == 0 {
		return m, nil
	}

	m.result = &Result{
		Action:          ActionHarnessInstall,
		InstallCommands: cmds,
	}

	return m, tea.Quit
}

// handleHarnessStatusesLoaded processes the loaded harness statuses.
func (m *model) handleHarnessStatusesLoaded(msg harnessStatusesLoadedMsg) (tea.Model, tea.Cmd) {
	m.homeHarness.loading = false
	m.homeHarness.statuses = msg.statuses

	return m, nil
}

// handleHarnessExpandHealth processes the health report for an expanded harness.
func (m *model) handleHarnessExpandHealth(msg harnessExpandHealthMsg) (tea.Model, tea.Cmd) {
	if m.activeScreen != screenHome || m.homeHarness.expanded < 0 {
		return m, nil
	}

	m.harnessExpand.loading = false
	m.harnessExpand.report = msg.report

	return m, nil
}
