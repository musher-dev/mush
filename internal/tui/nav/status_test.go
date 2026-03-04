package nav

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/harness"
)

func TestStatusHotkeyActivatesScreen(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if mdl.activeScreen != screenStatus {
		t.Errorf("activeScreen = %d, want screenStatus", mdl.activeScreen)
	}

	if !mdl.status.loading {
		t.Error("status.loading should be true after hotkey activation")
	}

	if !mdl.status.harnessLoading {
		t.Error("status.harnessLoading should be true after hotkey activation")
	}

	if mdl.cursor != 3 {
		t.Errorf("cursor = %d, want 3", mdl.cursor)
	}
}

func TestStatusChecksCompleteMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = true

	msg := statusChecksCompleteMsg{
		results: []doctor.Result{
			{Name: "API Connectivity", Status: doctor.StatusPass, Message: "ok"},
			{Name: "Authentication", Status: doctor.StatusFail, Message: "not authenticated"},
		},
		passed:   1,
		failed:   1,
		warnings: 0,
	}

	mdl = updateModel(mdl, msg)

	if mdl.status.loading {
		t.Error("status.loading should be false after completion")
	}

	if len(mdl.status.results) != 2 {
		t.Errorf("results = %d, want 2", len(mdl.status.results))
	}

	if mdl.status.passed != 1 {
		t.Errorf("passed = %d, want 1", mdl.status.passed)
	}

	if mdl.status.failed != 1 {
		t.Errorf("failed = %d, want 1", mdl.status.failed)
	}
}

func TestStatusEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenStatus)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestStatusRetryReRunsChecks(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = false
	mdl.status.results = []doctor.Result{
		{Name: "Test", Status: doctor.StatusPass, Message: "ok"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.status.loading {
		t.Error("status.loading should be true after retry")
	}

	if !mdl.status.harnessLoading {
		t.Error("status.harnessLoading should be true after retry")
	}

	if mdl.status.results != nil {
		t.Error("results should be nil during re-run")
	}

	if mdl.status.harnessReports != nil {
		t.Error("harnessReports should be nil during re-run")
	}
}

func TestStatusRetryIgnoredDuringLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = true

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.status.loading {
		t.Error("status.loading should remain true when retry pressed during loading")
	}
}

func TestStatusRetryIgnoredDuringHarnessLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = true

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.status.harnessLoading {
		t.Error("status.harnessLoading should remain true when retry pressed during loading")
	}
}

func TestStatusScreenView(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.results = []doctor.Result{
		{Name: "API Connectivity", Status: doctor.StatusPass, Message: "https://api.musher.dev (150ms)"},
		{Name: "Authentication", Status: doctor.StatusFail, Message: "Not authenticated", Detail: "Run 'mush auth login'"},
	}
	mdl.status.passed = 1
	mdl.status.failed = 1

	view := mdl.View()

	if !strings.Contains(view, "Diagnostics") {
		t.Error("view should contain 'Diagnostics' panel title")
	}

	if !strings.Contains(view, "API Connectivity") {
		t.Error("view should contain check name 'API Connectivity'")
	}

	if !strings.Contains(view, "Authentication") {
		t.Error("view should contain check name 'Authentication'")
	}

	if !strings.Contains(view, "1 passed") {
		t.Error("view should contain '1 passed' in summary")
	}

	if !strings.Contains(view, "1 failed") {
		t.Error("view should contain '1 failed' in summary")
	}
}

func TestStatusScreenViewLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = true

	view := mdl.View()

	if !strings.Contains(view, "Diagnostics") {
		t.Error("view should contain 'Diagnostics' panel title")
	}

	if !strings.Contains(view, "Running diagnostics") {
		t.Error("view should contain 'Running diagnostics' during loading")
	}
}

func TestStatusStaleResultIgnored(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHome // Not on status screen.
	mdl.status.loading = false

	msg := statusChecksCompleteMsg{
		results: []doctor.Result{
			{Name: "Test", Status: doctor.StatusPass, Message: "ok"},
		},
		passed: 1,
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.status.results) != 0 {
		t.Errorf("results should be empty when on different screen, got %d", len(mdl.status.results))
	}
}

func TestStatusScreenShowsHarnessPanel(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = false
	mdl.status.harnessReports = []*harness.HealthReport{
		{
			ProviderName: "claude",
			DisplayName:  "Claude Code",
			InstallHint:  "npm install -g @anthropic-ai/claude-code",
			Results: []harness.HealthResult{
				{Check: "Binary", Message: "claude found at /usr/local/bin/claude", Status: harness.HealthPass},
				{Check: "Version", Message: "2.1.63", Status: harness.HealthPass},
			},
		},
	}

	view := mdl.View()

	if !strings.Contains(view, "Harnesses") {
		t.Error("view should contain 'Harnesses' panel title")
	}

	if !strings.Contains(view, "Claude Code") {
		t.Error("view should contain provider display name 'Claude Code'")
	}

	if !strings.Contains(view, "Binary") {
		t.Error("view should contain check name 'Binary'")
	}
}

func TestHarnessHealthCompleteMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.harnessLoading = true

	msg := harnessHealthCompleteMsg{
		reports: []*harness.HealthReport{
			{
				ProviderName: "claude",
				DisplayName:  "Claude Code",
				Results: []harness.HealthResult{
					{Check: "Binary", Message: "found", Status: harness.HealthPass},
				},
			},
		},
	}

	mdl = updateModel(mdl, msg)

	if mdl.status.harnessLoading {
		t.Error("harnessLoading should be false after completion")
	}

	if len(mdl.status.harnessReports) != 1 {
		t.Errorf("harnessReports = %d, want 1", len(mdl.status.harnessReports))
	}
}

func TestStatusRetryReRunsBothPanels(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = false
	mdl.status.results = []doctor.Result{{Name: "Test", Status: doctor.StatusPass, Message: "ok"}}
	mdl.status.harnessReports = []*harness.HealthReport{{ProviderName: "claude"}}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.status.loading {
		t.Error("loading should be true after retry")
	}

	if !mdl.status.harnessLoading {
		t.Error("harnessLoading should be true after retry")
	}

	if mdl.status.results != nil {
		t.Error("results should be nil after retry")
	}

	if mdl.status.harnessReports != nil {
		t.Error("harnessReports should be nil after retry")
	}
}

func TestHarnessLoadingSpinner(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = true

	view := mdl.View()

	if !strings.Contains(view, "Checking harnesses") {
		t.Error("view should contain 'Checking harnesses' during harness loading")
	}
}

func TestStaleHarnessResultIgnored(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHome // Not on status screen.
	mdl.status.harnessLoading = true

	msg := harnessHealthCompleteMsg{
		reports: []*harness.HealthReport{
			{ProviderName: "claude"},
		},
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.status.harnessReports) != 0 {
		t.Error("harnessReports should be empty when on different screen")
	}
}

func TestInstallKeyCollectsMissingHarnesses(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = false
	mdl.status.harnessReports = []*harness.HealthReport{
		{
			ProviderName: "codex",
			DisplayName:  "Codex CLI",
			Results: []harness.HealthResult{
				{Check: "Binary", Message: "codex not found", Status: harness.HealthFail},
			},
		},
	}

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}

	if mdl.result == nil {
		t.Fatal("expected result to be set")
	}

	if mdl.result.Action != ActionHarnessInstall {
		t.Errorf("action = %d, want ActionHarnessInstall", mdl.result.Action)
	}

	if len(mdl.result.InstallCommands) == 0 {
		t.Error("expected install commands to be collected")
	}
}

func TestInstallKeyNopWhenAllInstalled(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = false
	mdl.status.harnessLoading = false
	mdl.status.harnessReports = []*harness.HealthReport{
		{
			ProviderName: "claude",
			DisplayName:  "Claude Code",
			Results: []harness.HealthResult{
				{Check: "Binary", Message: "found", Status: harness.HealthPass},
			},
		},
	}

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if cmd != nil {
		t.Error("expected no command when all harnesses are installed")
	}

	if mdl.result != nil {
		t.Error("expected result to be nil when all harnesses are installed")
	}
}

func TestInstallKeyIgnoredDuringLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenStatus
	mdl.status.loading = true

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if cmd != nil {
		t.Error("expected no command when loading")
	}
}
