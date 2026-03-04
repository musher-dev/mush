package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/harness"
)

// handleStatusKey processes key events on the status screen.
func (m *model) handleStatusKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Retry):
		if m.status.loading || m.status.harnessLoading {
			return m, nil
		}

		m.status.loading = true
		m.status.harnessLoading = true
		m.status.results = nil
		m.status.harnessReports = nil

		return m, tea.Batch(m.status.spinner.Tick, cmdRunStatusChecks(), cmdRunHarnessHealthChecks())

	case key.Matches(msg, m.keys.Install):
		if m.status.loading || m.status.harnessLoading {
			return m, nil
		}

		cmds := collectMissingInstallCommands(m.status.harnessReports)
		if len(cmds) == 0 {
			return m, nil
		}

		m.result = &Result{
			Action:          ActionHarnessInstall,
			InstallCommands: cmds,
		}

		return m, tea.Quit
	}

	return m, nil
}

// handleStatusChecksComplete processes the results of a diagnostic run.
func (m *model) handleStatusChecksComplete(msg statusChecksCompleteMsg) (tea.Model, tea.Cmd) {
	if m.activeScreen != screenStatus {
		return m, nil
	}

	m.status.loading = false
	m.status.results = msg.results
	m.status.passed = msg.passed
	m.status.failed = msg.failed
	m.status.warnings = msg.warnings

	return m, nil
}

// handleHarnessHealthComplete processes the results of harness health checks.
func (m *model) handleHarnessHealthComplete(msg harnessHealthCompleteMsg) (tea.Model, tea.Cmd) {
	if m.activeScreen != screenStatus {
		return m, nil
	}

	m.status.harnessLoading = false
	m.status.harnessReports = msg.reports

	return m, nil
}

// collectMissingInstallCommands gathers install commands for harnesses whose binary check failed.
func collectMissingInstallCommands(reports []*harness.HealthReport) [][]string {
	var cmds [][]string

	for _, r := range reports {
		if len(r.Results) == 0 {
			continue
		}

		// Binary check is always the first result.
		if r.Results[0].Status != harness.HealthFail {
			continue
		}

		// Look up the provider to get the install command.
		spec, ok := harness.GetProvider(r.ProviderName)
		if !ok || spec.Status == nil || len(spec.Status.InstallCommand) == 0 {
			continue
		}

		cmds = append(cmds, spec.Status.InstallCommand)
	}

	return cmds
}
