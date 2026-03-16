package nav

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/executil"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
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
func cmdLoadHarnessStatuses(ctx context.Context) tea.Cmd {
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
			if _, err := executil.LookPath(spec.Binary); err == nil {
				status.installed = true
				status.version = harnessQuickStatusVersion(detectVersion(ctx, spec))
			}

			statuses = append(statuses, status)
		}

		return harnessStatusesLoadedMsg{statuses: statuses}
	}
}

// detectVersion runs the provider's version command and returns the first line.
func detectVersion(ctx context.Context, spec *harnesstype.ProviderSpec) string {
	if spec.Status == nil || len(spec.Status.VersionArgs) == 0 {
		return ""
	}

	versionCtx, cancel := navStatusCtx(ctx)
	defer cancel()

	cmd, err := executil.CommandContext(versionCtx, spec.Binary, spec.Status.VersionArgs...)
	if err != nil {
		return ""
	}

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(strings.SplitN(string(out), "\n", maxSplitParts)[0])
}

// cmdRunSingleHarnessHealthCheck runs health checks for a single harness.
func cmdRunSingleHarnessHealthCheck(ctx context.Context, name string) tea.Cmd {
	return func() tea.Msg {
		checkCtx, cancel := navStatusCtx(ctx)
		defer cancel()

		report, _ := harness.CheckHealth(checkCtx, name)

		return harnessExpandHealthMsg{report: report}
	}
}
