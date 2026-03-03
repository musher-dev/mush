package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Key handlers ---

// handleWorkerListKey is a common key handler for worker list screens
// (habitats, queues). It manages cursor movement, back navigation, and
// selection. The cursor pointer and count are passed in so the handler is
// reusable without duplicating the switch structure.
func (m *model) handleWorkerListKey(
	msg tea.KeyMsg,
	cursor *int,
	count int,
	loading bool,
	onSelect func() (tea.Model, tea.Cmd),
) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Down):
		if !loading && *cursor < count-1 {
			*cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if !loading && *cursor > 0 {
			*cursor--
		}

	case key.Matches(msg, m.keys.Select):
		if !loading && count > 0 {
			return onSelect()
		}
	}

	return m, nil
}

// handleWorkerHabitatsKey processes key events on the habitat selection screen.
func (m *model) handleWorkerHabitatsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleWorkerListKey(msg,
		&m.workerHabitats.cursor,
		len(m.workerHabitats.habitats),
		m.workerHabitats.loading,
		m.selectHabitat,
	)
}

// selectHabitat transitions from habitat selection to queue loading.
func (m *model) selectHabitat() (tea.Model, tea.Cmd) {
	h := m.workerHabitats.habitats[m.workerHabitats.cursor]

	m.workerQueues = workerQueuesState{
		spinner:     m.workerQueues.spinner,
		loading:     true,
		habitatID:   h.ID,
		habitatName: h.Name,
	}

	m.pushScreen(screenWorkerQueues)

	return m, tea.Batch(
		m.workerQueues.spinner.Tick,
		cmdListQueues(m.deps.Client, h.ID, h.Name),
	)
}

// handleWorkerQueuesKey processes key events on the queue selection screen.
func (m *model) handleWorkerQueuesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleWorkerListKey(msg,
		&m.workerQueues.cursor,
		len(m.workerQueues.queues),
		m.workerQueues.loading,
		m.selectQueue,
	)
}

// selectQueue transitions from queue selection to harness selection.
func (m *model) selectQueue() (tea.Model, tea.Cmd) {
	selected := m.workerQueues.queues[m.workerQueues.cursor]

	m.workerHarness = workerHarnessState{
		habitatID:   m.workerQueues.habitatID,
		habitatName: m.workerQueues.habitatName,
		queueID:     selected.ID,
		queueName:   selected.Name,
	}

	m.pushScreen(screenWorkerHarness)

	return m, nil
}

// handleWorkerHarnessKey processes key events on the harness selection screen.
func (m *model) handleWorkerHarnessKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleWorkerListKey(msg,
		&m.workerHarness.cursor,
		len(m.harnesses),
		false, // harness list is local, never loading
		m.selectWorkerHarness,
	)
}

// selectWorkerHarness transitions from harness selection to instruction checking.
func (m *model) selectWorkerHarness() (tea.Model, tea.Cmd) {
	harness := m.harnesses[m.workerHarness.cursor].name

	m.workerChecking = workerCheckingState{
		spinner:     m.workerChecking.spinner,
		habitatID:   m.workerHarness.habitatID,
		habitatName: m.workerHarness.habitatName,
		queueID:     m.workerHarness.queueID,
		queueName:   m.workerHarness.queueName,
		harness:     harness,
	}

	m.pushScreen(screenWorkerChecking)

	return m, tea.Batch(
		m.workerChecking.spinner.Tick,
		cmdCheckInstructions(m.deps.Client, m.workerHarness.queueID),
	)
}

// handleWorkerCheckingKey processes key events on the checking spinner screen.
func (m *model) handleWorkerCheckingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) {
		m.popScreen()
	}

	return m, nil
}

// handleWorkerConfirmKey processes key events on the confirmation screen.
func (m *model) handleWorkerConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Tab):
		m.workerConfirm.buttonIdx = (m.workerConfirm.buttonIdx + 1) % 2 //nolint:mnd // 2 buttons

	case key.Matches(msg, m.keys.Select):
		if m.workerConfirm.buttonIdx == 1 {
			// Cancel — go back.
			m.popScreen()

			return m, nil
		}

		// Start — set result and quit.
		m.result = &Result{
			Action:             ActionWorkerStart,
			HabitatID:          m.workerConfirm.habitatID,
			QueueID:            m.workerConfirm.queueID,
			QueueName:          m.workerConfirm.queueName,
			SupportedHarnesses: []string{m.workerConfirm.harness},
		}

		return m, tea.Quit
	}

	return m, nil
}

// handleWorkerErrorKey processes key events on the worker error screen.
func (m *model) handleWorkerErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Retry), key.Matches(msg, m.keys.Select):
		return m.retryWorker()
	}

	return m, nil
}

// retryWorker retries the failed worker step based on retryAction.
func (m *model) retryWorker() (tea.Model, tea.Cmd) {
	if m.deps == nil || m.deps.Client == nil {
		return m, nil
	}

	switch m.workerError.retryAction {
	case "habitats":
		m.workerHabitats = workerHabitatsState{
			spinner: m.workerHabitats.spinner,
			loading: true,
		}

		m.activeScreen = screenWorkerHabitats

		return m, tea.Batch(
			m.workerHabitats.spinner.Tick,
			cmdListHabitats(m.deps.Client),
		)

	case "queues":
		m.workerQueues = workerQueuesState{
			spinner:     m.workerQueues.spinner,
			loading:     true,
			habitatID:   m.workerError.habitatID,
			habitatName: m.workerError.habitatName,
		}

		m.activeScreen = screenWorkerQueues

		return m, tea.Batch(
			m.workerQueues.spinner.Tick,
			cmdListQueues(m.deps.Client, m.workerError.habitatID, m.workerError.habitatName),
		)

	case "instructions":
		m.workerChecking = workerCheckingState{
			spinner:     m.workerChecking.spinner,
			habitatID:   m.workerError.habitatID,
			habitatName: m.workerError.habitatName,
			queueID:     m.workerError.queueID,
			queueName:   m.workerError.queueName,
			harness:     m.workerError.harness,
		}

		m.activeScreen = screenWorkerChecking

		return m, tea.Batch(
			m.workerChecking.spinner.Tick,
			cmdCheckInstructions(m.deps.Client, m.workerError.queueID),
		)
	}

	return m, nil
}

// --- Message handlers ---

// handleWorkerHabitatsLoaded processes a successful habitat list.
func (m *model) handleWorkerHabitatsLoaded(msg workerHabitatsLoadedMsg) (tea.Model, tea.Cmd) {
	if len(msg.habitats) == 0 {
		m.workerError = workerErrorState{
			message:     "No habitats available",
			hint:        "Create a habitat in the Musher web UI first",
			retryAction: "habitats",
		}

		m.activeScreen = screenWorkerError

		return m, nil
	}

	m.workerHabitats.loading = false
	m.workerHabitats.habitats = msg.habitats
	m.workerHabitats.cursor = 0

	return m, nil
}

// handleWorkerHabitatsError processes a habitat list error.
func (m *model) handleWorkerHabitatsError(msg workerHabitatsErrorMsg) (tea.Model, tea.Cmd) {
	m.workerError = workerErrorState{
		message:     msg.err.Error(),
		hint:        "Check your network connection and API credentials",
		retryAction: "habitats",
	}

	m.activeScreen = screenWorkerError

	return m, nil
}

// handleWorkerQueuesLoaded processes a successful queue list.
func (m *model) handleWorkerQueuesLoaded(msg workerQueuesLoadedMsg) (tea.Model, tea.Cmd) {
	if len(msg.queues) == 0 {
		m.workerError = workerErrorState{
			message:     "No queues available for this habitat",
			hint:        "Create a queue in the Musher web UI first",
			retryAction: "queues",
			habitatID:   msg.habitatID,
			habitatName: msg.habitatName,
		}

		m.activeScreen = screenWorkerError

		return m, nil
	}

	m.workerQueues.loading = false
	m.workerQueues.queues = msg.queues
	m.workerQueues.cursor = 0

	return m, nil
}

// handleWorkerQueuesError processes a queue list error.
func (m *model) handleWorkerQueuesError(msg workerQueuesErrorMsg) (tea.Model, tea.Cmd) {
	m.workerError = workerErrorState{
		message:     msg.err.Error(),
		hint:        "Check your network connection and API credentials",
		retryAction: "queues",
		habitatID:   msg.habitatID,
		habitatName: msg.habitatName,
	}

	m.activeScreen = screenWorkerError

	return m, nil
}

// handleWorkerInstructionCheck processes an instruction availability result.
func (m *model) handleWorkerInstructionCheck(msg workerInstructionCheckMsg) (tea.Model, tea.Cmd) {
	if !msg.available {
		m.workerError = workerErrorState{
			message:     "No active instructions for this queue",
			hint:        "Configure and activate an instruction in the Musher web UI",
			retryAction: "instructions",
			habitatID:   m.workerChecking.habitatID,
			habitatName: m.workerChecking.habitatName,
			queueID:     m.workerChecking.queueID,
			queueName:   m.workerChecking.queueName,
			harness:     m.workerChecking.harness,
		}

		m.activeScreen = screenWorkerError

		return m, nil
	}

	m.workerConfirm = workerConfirmState{
		habitatID:   m.workerChecking.habitatID,
		habitatName: m.workerChecking.habitatName,
		queueID:     m.workerChecking.queueID,
		queueName:   m.workerChecking.queueName,
		harness:     m.workerChecking.harness,
	}

	m.activeScreen = screenWorkerConfirm

	return m, nil
}

// handleWorkerInstructionError processes an instruction check error.
func (m *model) handleWorkerInstructionError(msg workerInstructionErrorMsg) (tea.Model, tea.Cmd) {
	m.workerError = workerErrorState{
		message:     msg.err.Error(),
		hint:        "Check your network connection or run 'mush doctor'",
		retryAction: "instructions",
		habitatID:   m.workerChecking.habitatID,
		habitatName: m.workerChecking.habitatName,
		queueID:     m.workerChecking.queueID,
		queueName:   m.workerChecking.queueName,
		harness:     m.workerChecking.harness,
	}

	m.activeScreen = screenWorkerError

	return m, nil
}
