package nav

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/harness"
)

// statusChecksCompleteMsg carries the results of a diagnostic run.
type statusChecksCompleteMsg struct {
	results  []doctor.Result
	passed   int
	failed   int
	warnings int
}

// cmdRunStatusChecks runs all diagnostic checks asynchronously.
func cmdRunStatusChecks(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		checkCtx, cancel := navStatusCtx(ctx)
		defer cancel()

		runner := doctor.New()
		results := runner.Run(checkCtx)
		passed, failed, warnings := doctor.Summary(results)

		return statusChecksCompleteMsg{
			results:  results,
			passed:   passed,
			failed:   failed,
			warnings: warnings,
		}
	}
}

// harnessHealthCompleteMsg carries the results of harness health checks.
type harnessHealthCompleteMsg struct {
	reports []*harness.HealthReport
}

// cmdRunHarnessHealthChecks runs health checks for all registered harnesses.
func cmdRunHarnessHealthChecks(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		checkCtx, cancel := navStatusCtx(ctx)
		defer cancel()

		reports := harness.CheckAllHealth(checkCtx)

		return harnessHealthCompleteMsg{reports: reports}
	}
}
