package nav

import (
	"context"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/harness"
)

// harnessStatusesLoadedMsg carries the quick statuses for all harnesses.
type harnessStatusesLoadedMsg struct {
	statuses []harnessQuickStatus
}

// harnessExpandHealthMsg carries the health report for a single expanded harness.
type harnessExpandHealthMsg struct {
	report *harness.HealthReport
}

// cmdLoadHarnessStatuses detects installed harnesses and their versions asynchronously.
func cmdLoadHarnessStatuses() tea.Cmd {
	return func() tea.Msg {
		names := harness.ProviderNames()
		statuses := make([]harnessQuickStatus, 0, len(names))

		for _, name := range names {
			spec, ok := harness.GetProvider(name)
			if !ok {
				continue
			}

			status := harnessQuickStatus{
				name:        spec.Name,
				displayName: spec.DisplayName,
			}

			// Check if binary is available.
			if _, err := exec.LookPath(spec.Binary); err == nil {
				status.installed = true
				status.version = harnessQuickStatusVersion(detectVersion(spec))
			}

			statuses = append(statuses, status)
		}

		return harnessStatusesLoadedMsg{statuses: statuses}
	}
}

// detectVersion runs the provider's version command and returns the first line.
func detectVersion(spec *harness.ProviderSpec) string {
	if spec.Status == nil || len(spec.Status.VersionArgs) == 0 {
		return ""
	}

	//nolint:gosec // args come from embedded YAML, not user input
	cmd := exec.CommandContext(context.Background(), spec.Binary, spec.Status.VersionArgs...)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]) //nolint:mnd // split into at most 2 parts
}

// cmdRunSingleHarnessHealthCheck runs health checks for a single harness.
func cmdRunSingleHarnessHealthCheck(name string) tea.Cmd {
	return func() tea.Msg {
		report, _ := harness.CheckHealth(context.Background(), name)
		return harnessExpandHealthMsg{report: report}
	}
}
