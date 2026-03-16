package main

import (
	"context"
	"fmt"

	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

type selectableItem[T any] struct {
	items          []T
	resolveByInput func(T, string) bool
	label          func(T) string
	promptSelect   func([]T, *output.Writer) (T, error)
	notFound       func(string) error
	noneAvailable  error
	required       error
	cancelMessage  string
	cancelHint     string
	selectError    string
}

func resolveSelectable[T any](
	input string,
	out *output.Writer,
	cfg selectableItem[T],
) (T, error) {
	var zero T

	if input != "" {
		for _, item := range cfg.items {
			if cfg.resolveByInput(item, input) {
				out.Print("%s\n", cfg.label(item))
				return item, nil
			}
		}

		return zero, cfg.notFound(input)
	}

	if len(cfg.items) == 0 {
		return zero, cfg.noneAvailable
	}

	if out.NoInput {
		if len(cfg.items) == 1 {
			out.Print("%s\n", cfg.label(cfg.items[0]))
			return cfg.items[0], nil
		}

		return zero, cfg.required
	}

	selected, err := cfg.promptSelect(cfg.items, out)
	if err != nil {
		if prompt.IsCanceled(err) {
			return zero, clierrors.New(clierrors.ExitUsage, cfg.cancelMessage).
				WithHint(cfg.cancelHint)
		}

		return zero, clierrors.Wrap(clierrors.ExitGeneral, cfg.selectError, err)
	}

	out.Print("%s\n", cfg.label(selected))

	return selected, nil
}

// resolveHabitatID determines the habitat ID to use.
func resolveHabitatID(ctx context.Context, c *client.Client, habitatFlag string, out *output.Writer) (string, error) {
	habitats, err := c.ListHabitats(ctx)
	if err != nil {
		return "", clierrors.Wrap(clierrors.ExitNetwork, "Failed to fetch habitats", err).
			WithHint("Check your network connection and API credentials")
	}

	selected, err := resolveSelectable(habitatFlag, out, selectableItem[client.HabitatSummary]{
		items: habitats,
		resolveByInput: func(item client.HabitatSummary, input string) bool {
			return item.Slug == input || item.ID == input
		},
		label: func(item client.HabitatSummary) string {
			return fmt.Sprintf("Connecting to habitat: %s (%s)", item.Name, item.Slug)
		},
		promptSelect: func(items []client.HabitatSummary, out *output.Writer) (client.HabitatSummary, error) {
			selected, promptErr := prompt.SelectHabitat(items, out)
			if promptErr != nil {
				return client.HabitatSummary{}, fmt.Errorf("prompt select habitat: %w", promptErr)
			}

			return *selected, nil
		},
		notFound: func(input string) error {
			return clierrors.HabitatNotFound(input)
		},
		noneAvailable: clierrors.NoHabitats(),
		required:      clierrors.HabitatRequired(),
		cancelMessage: "Habitat selection canceled",
		cancelHint:    "Pass --habitat to select non-interactively",
		selectError:   "Failed to select habitat",
	})
	if err != nil {
		return "", err
	}

	return selected.ID, nil
}

// resolveQueue determines the queue to use.
func resolveQueue(
	ctx context.Context,
	c *client.Client,
	habitatID string,
	queueFlag string,
	out *output.Writer,
) (client.QueueSummary, error) {
	queues, err := c.ListQueues(ctx, habitatID)
	if err != nil {
		return client.QueueSummary{}, clierrors.Wrap(clierrors.ExitNetwork, "Failed to fetch queues", err).
			WithHint("Check your network connection and API credentials")
	}

	return resolveSelectable(queueFlag, out, selectableItem[client.QueueSummary]{
		items: queues,
		resolveByInput: func(item client.QueueSummary, input string) bool {
			return item.ID == input || item.Slug == input
		},
		label: func(item client.QueueSummary) string {
			return fmt.Sprintf("Filtering by queue: %s (%s)", item.Name, item.Slug)
		},
		promptSelect: func(items []client.QueueSummary, out *output.Writer) (client.QueueSummary, error) {
			selected, promptErr := prompt.SelectQueue(items, out)
			if promptErr != nil {
				return client.QueueSummary{}, fmt.Errorf("prompt select queue: %w", promptErr)
			}

			return *selected, nil
		},
		notFound: func(input string) error {
			return clierrors.QueueNotFound(input)
		},
		noneAvailable: clierrors.NoQueuesForHabitat(),
		required:      clierrors.QueueRequired(),
		cancelMessage: "Queue selection canceled",
		cancelHint:    "Pass --queue to select non-interactively",
		selectError:   "Failed to select queue",
	})
}
