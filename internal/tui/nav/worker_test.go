package nav

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

func TestWorkerHotkeyNoClientShowsError(t *testing.T) {
	t.Parallel()

	mdl := testModel() // nil deps → no client

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	if mdl.activeScreen != screenWorkerError {
		t.Fatalf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "Not authenticated") {
		t.Errorf("error message = %q, want to contain 'Not authenticated'", mdl.workerError.message)
	}
}

func TestWorkerHotkeyWithClientShowsHabitats(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{Client: client.New("http://localhost", "test-key")})

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

	if mdl.activeScreen != screenWorkerHabitats {
		t.Fatalf("activeScreen = %d, want screenWorkerHabitats", mdl.activeScreen)
	}

	if !mdl.workerHabitats.loading {
		t.Error("workerHabitats.loading should be true")
	}
}

func TestWorkerHabitatsLoadedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerHabitats
	mdl.workerHabitats.loading = true

	msg := workerHabitatsLoadedMsg{
		habitats: []client.HabitatSummary{
			{ID: "h1", Name: "Production", Slug: "prod"},
			{ID: "h2", Name: "Staging", Slug: "staging"},
		},
	}

	mdl = updateModel(mdl, msg)

	if mdl.workerHabitats.loading {
		t.Error("loading should be false after habitats loaded")
	}

	if len(mdl.workerHabitats.habitats) != 2 {
		t.Errorf("habitats count = %d, want 2", len(mdl.workerHabitats.habitats))
	}

	if mdl.workerHabitats.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.workerHabitats.cursor)
	}
}

func TestWorkerHabitatsEmptyShowsError(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerHabitats
	mdl.workerHabitats.loading = true

	mdl = updateModel(mdl, workerHabitatsLoadedMsg{habitats: nil})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "No habitats") {
		t.Errorf("error message = %q, want to contain 'No habitats'", mdl.workerError.message)
	}
}

func TestWorkerHabitatsErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerHabitats

	mdl = updateModel(mdl, workerHabitatsErrorMsg{err: fmt.Errorf("network timeout")})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "network timeout") {
		t.Errorf("error message = %q, want to contain 'network timeout'", mdl.workerError.message)
	}

	if mdl.workerError.retryAction != "habitats" {
		t.Errorf("retryAction = %q, want 'habitats'", mdl.workerError.retryAction)
	}
}

func TestWorkerHabitatNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerHabitats
	mdl.workerHabitats.habitats = []client.HabitatSummary{
		{ID: "h1", Name: "A"},
		{ID: "h2", Name: "B"},
		{ID: "h3", Name: "C"},
	}

	if mdl.workerHabitats.cursor != 0 {
		t.Fatalf("cursor should start at 0")
	}

	// Down.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerHabitats.cursor != 1 {
		t.Errorf("cursor = %d, want 1", mdl.workerHabitats.cursor)
	}

	// Down again.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerHabitats.cursor != 2 {
		t.Errorf("cursor = %d, want 2", mdl.workerHabitats.cursor)
	}

	// Down clamped.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerHabitats.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", mdl.workerHabitats.cursor)
	}

	// Up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})
	if mdl.workerHabitats.cursor != 1 {
		t.Errorf("cursor = %d, want 1", mdl.workerHabitats.cursor)
	}
}

func TestWorkerHabitatSelectTransitionsToQueues(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{Client: client.New("http://localhost", "test-key")})
	mdl.activeScreen = screenWorkerHabitats
	mdl.workerHabitats.habitats = []client.HabitatSummary{
		{ID: "h1", Name: "Production", Slug: "prod"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenWorkerQueues {
		t.Errorf("activeScreen = %d, want screenWorkerQueues", mdl.activeScreen)
	}

	if !mdl.workerQueues.loading {
		t.Error("workerQueues.loading should be true")
	}

	if mdl.workerQueues.habitatID != "h1" {
		t.Errorf("habitatID = %q, want 'h1'", mdl.workerQueues.habitatID)
	}

	if mdl.workerQueues.habitatName != "Production" {
		t.Errorf("habitatName = %q, want 'Production'", mdl.workerQueues.habitatName)
	}
}

func TestWorkerHabitatsEscGoesHome(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerHabitats)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestWorkerQueuesLoadedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerQueues
	mdl.workerQueues.loading = true

	msg := workerQueuesLoadedMsg{
		queues: []client.QueueSummary{
			{ID: "q1", Name: "Default", Slug: "default"},
			{ID: "q2", Name: "Priority", Slug: "priority"},
		},
		habitatID:   "h1",
		habitatName: "Production",
	}

	mdl = updateModel(mdl, msg)

	if mdl.workerQueues.loading {
		t.Error("loading should be false")
	}

	if len(mdl.workerQueues.queues) != 2 {
		t.Errorf("queues count = %d, want 2", len(mdl.workerQueues.queues))
	}
}

func TestWorkerQueuesEmptyShowsError(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerQueues
	mdl.workerQueues.loading = true

	mdl = updateModel(mdl, workerQueuesLoadedMsg{queues: nil, habitatID: "h1", habitatName: "Prod"})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "No queues") {
		t.Errorf("error message = %q, want to contain 'No queues'", mdl.workerError.message)
	}
}

func TestWorkerQueuesErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerQueues

	mdl = updateModel(mdl, workerQueuesErrorMsg{
		err:         fmt.Errorf("forbidden"),
		habitatID:   "h1",
		habitatName: "Prod",
	})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if mdl.workerError.retryAction != "queues" {
		t.Errorf("retryAction = %q, want 'queues'", mdl.workerError.retryAction)
	}
}

func TestWorkerQueueNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerQueues
	mdl.workerQueues.queues = []client.QueueSummary{
		{ID: "q1", Name: "A"},
		{ID: "q2", Name: "B"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerQueues.cursor != 1 {
		t.Errorf("cursor = %d, want 1", mdl.workerQueues.cursor)
	}

	// Clamped.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerQueues.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (clamped)", mdl.workerQueues.cursor)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})
	if mdl.workerQueues.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.workerQueues.cursor)
	}
}

func TestWorkerQueueSelectTransitionsToHarness(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{Client: client.New("http://localhost", "test-key")})
	mdl.activeScreen = screenWorkerQueues
	mdl.workerQueues.queues = []client.QueueSummary{
		{ID: "q1", Name: "Default", Slug: "default"},
	}
	mdl.workerQueues.habitatID = "h1"
	mdl.workerQueues.habitatName = "Production"

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenWorkerHarness {
		t.Errorf("activeScreen = %d, want screenWorkerHarness", mdl.activeScreen)
	}

	if mdl.workerHarness.queueID != "q1" {
		t.Errorf("queueID = %q, want 'q1'", mdl.workerHarness.queueID)
	}

	if mdl.workerHarness.habitatID != "h1" {
		t.Errorf("habitatID = %q, want 'h1'", mdl.workerHarness.habitatID)
	}
}

func TestWorkerQueuesEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerQueues)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestWorkerHarnessNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerHarness

	if len(mdl.harnesses) < 2 {
		t.Skipf("need at least 2 harnesses, got %d", len(mdl.harnesses))
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	if mdl.workerHarness.cursor != 1 {
		t.Errorf("cursor = %d, want 1", mdl.workerHarness.cursor)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})
	if mdl.workerHarness.cursor != 0 {
		t.Errorf("cursor = %d, want 0", mdl.workerHarness.cursor)
	}
}

func TestWorkerHarnessSelectTransitionsToChecking(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{Client: client.New("http://localhost", "test-key")})
	mdl.activeScreen = screenWorkerHarness
	mdl.workerHarness = workerHarnessState{
		habitatID:   "h1",
		habitatName: "Prod",
		queueID:     "q1",
		queueName:   "Default",
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenWorkerChecking {
		t.Errorf("activeScreen = %d, want screenWorkerChecking", mdl.activeScreen)
	}

	if mdl.workerChecking.queueID != "q1" {
		t.Errorf("queueID = %q, want 'q1'", mdl.workerChecking.queueID)
	}

	if mdl.workerChecking.harness == "" {
		t.Error("harness should be set from selected harness")
	}
}

func TestWorkerHarnessEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerHarness)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestWorkerInstructionCheckAvailable(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerChecking
	mdl.workerChecking = workerCheckingState{
		habitatID:   "h1",
		habitatName: "Prod",
		queueID:     "q1",
		queueName:   "Default",
		harness:     "claude",
	}

	mdl = updateModel(mdl, workerInstructionCheckMsg{available: true, instructionName: "test-instr"})

	if mdl.activeScreen != screenWorkerConfirm {
		t.Errorf("activeScreen = %d, want screenWorkerConfirm", mdl.activeScreen)
	}

	if mdl.workerConfirm.harness != "claude" {
		t.Errorf("harness = %q, want 'claude'", mdl.workerConfirm.harness)
	}

	if mdl.workerConfirm.queueName != "Default" {
		t.Errorf("queueName = %q, want 'Default'", mdl.workerConfirm.queueName)
	}
}

func TestWorkerInstructionCheckUnavailable(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerChecking
	mdl.workerChecking = workerCheckingState{
		habitatID: "h1",
		queueID:   "q1",
		harness:   "claude",
	}

	mdl = updateModel(mdl, workerInstructionCheckMsg{available: false})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "No active instructions") {
		t.Errorf("error message = %q, want to contain 'No active instructions'", mdl.workerError.message)
	}

	if mdl.workerError.retryAction != "instructions" {
		t.Errorf("retryAction = %q, want 'instructions'", mdl.workerError.retryAction)
	}
}

func TestWorkerInstructionErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerChecking
	mdl.workerChecking = workerCheckingState{
		habitatID: "h1",
		queueID:   "q1",
		harness:   "claude",
	}

	mdl = updateModel(mdl, workerInstructionErrorMsg{err: fmt.Errorf("server error")})

	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.workerError.message, "server error") {
		t.Errorf("error message = %q, want to contain 'server error'", mdl.workerError.message)
	}
}

func TestWorkerConfirmTabSwitchesButtons(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenWorkerConfirm
	mdl.workerConfirm = workerConfirmState{
		habitatName: "Prod",
		queueName:   "Default",
		harness:     "claude",
		buttonIdx:   0,
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})
	if mdl.workerConfirm.buttonIdx != 1 {
		t.Errorf("buttonIdx = %d, want 1", mdl.workerConfirm.buttonIdx)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})
	if mdl.workerConfirm.buttonIdx != 0 {
		t.Errorf("buttonIdx = %d, want 0", mdl.workerConfirm.buttonIdx)
	}
}

func TestWorkerConfirmCancelGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerConfirm)
	mdl.workerConfirm = workerConfirmState{
		buttonIdx: 1, // Cancel
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after cancel", mdl.activeScreen)
	}
}

func TestWorkerConfirmStartSetsResult(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerConfirm)
	mdl.workerConfirm = workerConfirmState{
		habitatID:   "h1",
		habitatName: "Prod",
		queueID:     "q1",
		queueName:   "Default",
		harness:     "claude",
		buttonIdx:   0, // Start
	}

	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.result == nil {
		t.Fatal("result should be set after start")
	}

	if mdl.result.Action != ActionWorkerStart {
		t.Errorf("result.Action = %d, want ActionWorkerStart", mdl.result.Action)
	}

	if mdl.result.HabitatID != "h1" {
		t.Errorf("result.HabitatID = %q, want 'h1'", mdl.result.HabitatID)
	}

	if mdl.result.QueueID != "q1" {
		t.Errorf("result.QueueID = %q, want 'q1'", mdl.result.QueueID)
	}

	if mdl.result.QueueName != "Default" {
		t.Errorf("result.QueueName = %q, want 'Default'", mdl.result.QueueName)
	}

	if len(mdl.result.SupportedHarnesses) != 1 || mdl.result.SupportedHarnesses[0] != "claude" {
		t.Errorf("result.SupportedHarnesses = %v, want [claude]", mdl.result.SupportedHarnesses)
	}

	if cmd == nil {
		t.Fatal("expected quit command")
	}

	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
	}
}

func TestWorkerConfirmEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerConfirm)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestWorkerErrorEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenWorkerError)
	mdl.workerError = workerErrorState{message: "error"}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestWorkerErrorRetryHabitats(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{Client: client.New("http://localhost", "test-key")})
	mdl.activeScreen = screenWorkerError
	mdl.workerError = workerErrorState{
		message:     "network error",
		retryAction: "habitats",
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenWorkerHabitats {
		t.Errorf("activeScreen = %d, want screenWorkerHabitats after retry", mdl.activeScreen)
	}

	if !mdl.workerHabitats.loading {
		t.Error("should be loading after retry")
	}
}

func TestWorkerErrorRetryNoClient(t *testing.T) {
	t.Parallel()

	mdl := testModel() // nil deps
	mdl.activeScreen = screenWorkerError
	mdl.workerError = workerErrorState{
		message:     "error",
		retryAction: "habitats",
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Without client, retry is a no-op.
	if mdl.activeScreen != screenWorkerError {
		t.Errorf("activeScreen = %d, want screenWorkerError (no client)", mdl.activeScreen)
	}
}

func TestWorkerScreenViews(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		screen screen
		setup  func(*model)
		check  string
	}{
		{
			name:   "habitats_loading",
			screen: screenWorkerHabitats,
			setup: func(m *model) {
				m.workerHabitats.loading = true
			},
			check: "Loading habitats",
		},
		{
			name:   "habitats_list",
			screen: screenWorkerHabitats,
			setup: func(m *model) {
				m.workerHabitats.habitats = []client.HabitatSummary{
					{ID: "h1", Name: "Production", Slug: "prod"},
				}
			},
			check: "Production",
		},
		{
			name:   "queues_loading",
			screen: screenWorkerQueues,
			setup: func(m *model) {
				m.workerQueues.loading = true
				m.workerQueues.habitatName = "Prod"
			},
			check: "Loading queues",
		},
		{
			name:   "queues_list",
			screen: screenWorkerQueues,
			setup: func(m *model) {
				m.workerQueues.queues = []client.QueueSummary{
					{ID: "q1", Name: "Default", Slug: "default"},
				}
				m.workerQueues.habitatName = "Prod"
			},
			check: "Default",
		},
		{
			name:   "harness",
			screen: screenWorkerHarness,
			setup: func(m *model) {
				m.workerHarness = workerHarnessState{
					habitatName: "Prod",
					queueName:   "Default",
				}
			},
			check: "Harness",
		},
		{
			name:   "checking",
			screen: screenWorkerChecking,
			setup: func(m *model) {
				m.workerChecking.queueName = "Default"
			},
			check: "Checking",
		},
		{
			name:   "confirm",
			screen: screenWorkerConfirm,
			setup: func(m *model) {
				m.workerConfirm = workerConfirmState{
					habitatName: "Prod",
					queueName:   "Default",
					harness:     "claude",
				}
			},
			check: "Confirm",
		},
		{
			name:   "error",
			screen: screenWorkerError,
			setup: func(m *model) {
				m.workerError = workerErrorState{message: "something failed"}
			},
			check: "Error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mdl := testModel()
			mdl.activeScreen = test.screen
			test.setup(mdl)

			view := mdl.View()
			if !strings.Contains(view, test.check) {
				t.Errorf("view for %s should contain %q", test.name, test.check)
			}
		})
	}
}
