package nav

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

// --- Worker message types ---

// workerHabitatsLoadedMsg carries a successful habitat list.
type workerHabitatsLoadedMsg struct {
	habitats []client.HabitatSummary
}

// workerHabitatsErrorMsg carries a habitat list error.
type workerHabitatsErrorMsg struct {
	err error
}

// workerQueuesLoadedMsg carries a successful queue list.
type workerQueuesLoadedMsg struct {
	queues      []client.QueueSummary
	habitatID   string
	habitatName string
}

// workerQueuesErrorMsg carries a queue list error.
type workerQueuesErrorMsg struct {
	err         error
	habitatID   string
	habitatName string
}

// workerInstructionCheckMsg carries an instruction availability result.
type workerInstructionCheckMsg struct {
	available       bool
	instructionName string
}

// workerInstructionErrorMsg carries an instruction check error.
type workerInstructionErrorMsg struct {
	err error
}

// --- Worker commands ---

// cmdListHabitats fetches the habitat list asynchronously.
func cmdListHabitats(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		habitats, err := c.ListHabitats(ctx)
		if err != nil {
			return workerHabitatsErrorMsg{err: err}
		}

		return workerHabitatsLoadedMsg{habitats: habitats}
	}
}

// cmdListQueues fetches the queue list for a habitat asynchronously.
func cmdListQueues(c *client.Client, habitatID, habitatName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		queues, err := c.ListQueues(ctx, habitatID)
		if err != nil {
			return workerQueuesErrorMsg{
				err:         err,
				habitatID:   habitatID,
				habitatName: habitatName,
			}
		}

		return workerQueuesLoadedMsg{
			queues:      queues,
			habitatID:   habitatID,
			habitatName: habitatName,
		}
	}
}

// cmdCheckInstructions checks instruction availability for a queue.
func cmdCheckInstructions(c *client.Client, queueID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		availability, err := c.GetQueueInstructionAvailability(ctx, queueID)
		if err != nil {
			return workerInstructionErrorMsg{err: err}
		}

		return workerInstructionCheckMsg{
			available:       availability != nil && availability.HasActiveInstruction,
			instructionName: instructionNameFromAvailability(availability),
		}
	}
}

// instructionNameFromAvailability extracts a display name from the availability response.
func instructionNameFromAvailability(avail *client.InstructionAvailability) string {
	if avail == nil {
		return ""
	}

	if avail.InstructionName != "" {
		return avail.InstructionName
	}

	return avail.InstructionSlug
}
