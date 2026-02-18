package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

// resolveHabitatID determines the habitat ID to use, either from the flag,
// non-interactive auto-selection, or interactive selection.
func resolveHabitatID(ctx context.Context, c *client.Client, habitatFlag string, out *output.Writer) (string, error) {
	// Priority: flag > non-interactive auto-selection > interactive selection
	if habitatFlag != "" {
		// Fetch habitats to resolve slug to ID
		habitats, err := c.ListHabitats(ctx)
		if err != nil {
			out.Failure("Failed to fetch habitats")
			return "", fmt.Errorf("fetch habitats: %w", err)
		}

		// Try to find by slug or ID
		for _, h := range habitats {
			if h.Slug == habitatFlag || h.ID == habitatFlag {
				out.Print("Linking to habitat: %s (%s)\n", h.Name, h.Slug)
				return h.ID, nil
			}
		}

		return "", clierrors.HabitatNotFound(habitatFlag)
	}

	// Interactive selection
	habitats, err := c.ListHabitats(ctx)
	if err != nil {
		out.Failure("Failed to fetch habitats")
		return "", fmt.Errorf("fetch habitats: %w", err)
	}

	if len(habitats) == 0 {
		return "", clierrors.NoHabitats()
	}

	// Non-interactive mode: auto-select single habitat or error
	if out.NoInput {
		if len(habitats) == 1 {
			out.Print("Linking to habitat: %s (%s)\n", habitats[0].Name, habitats[0].Slug)
			return habitats[0].ID, nil
		}

		return "", clierrors.HabitatRequired()
	}

	// Interactive mode: always prompt
	selected, err := prompt.SelectHabitat(habitats, out)
	if err != nil {
		return "", fmt.Errorf("select habitat: %w", err)
	}

	out.Print("Linking to habitat: %s (%s)\n", selected.Name, selected.Slug)

	return selected.ID, nil
}

// resolveQueue determines the queue to use, either from flag validation or interactive selection.
func resolveQueue(
	ctx context.Context,
	c *client.Client,
	habitatID string,
	queueFlag string,
	out *output.Writer,
) (client.QueueSummary, error) {
	queues, err := c.ListQueues(ctx, habitatID)
	if err != nil {
		out.Failure("Failed to fetch queues")
		return client.QueueSummary{}, fmt.Errorf("fetch queues: %w", err)
	}

	if queueFlag != "" {
		for _, q := range queues {
			if q.ID == queueFlag || q.Slug == queueFlag {
				out.Print("Filtering by queue: %s (%s)\n", q.Name, q.Slug)
				return q, nil
			}
		}

		return client.QueueSummary{}, clierrors.QueueNotFound(queueFlag)
	}

	if len(queues) == 0 {
		return client.QueueSummary{}, clierrors.NoQueuesForHabitat()
	}

	// Non-interactive mode: auto-select single queue or error
	if out.NoInput {
		if len(queues) == 1 {
			out.Print("Filtering by queue: %s (%s)\n", queues[0].Name, queues[0].Slug)
			return queues[0], nil
		}

		return client.QueueSummary{}, clierrors.QueueRequired()
	}

	// Interactive mode: always prompt
	selected, err := prompt.SelectQueue(queues, out)
	if err != nil {
		return client.QueueSummary{}, fmt.Errorf("select queue: %w", err)
	}

	out.Print("Filtering by queue: %s (%s)\n", selected.Name, selected.Slug)

	return *selected, nil
}

// LinkStatus represents link status for JSON output.
type LinkStatus struct {
	Source     string `json:"source"`
	Credential string `json:"credential"`
	Workspace  string `json:"workspace"`
	Active     bool   `json:"active"`
}

func newLinkStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show link status",
		Long:  `Show the current link status, authenticated identity, and habitat information.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			// Get credentials and create client
			source, c, err := apiClientFactory()
			if err != nil {
				return err
			}

			// Validate connection with spinner
			spin := out.Spinner("Checking connection")
			spin.Start()

			identity, err := c.ValidateKey(cmd.Context())
			if err != nil {
				spin.StopWithFailure("Authentication failed")
				return clierrors.AuthFailed(err)
			}

			spin.StopWithSuccess("Connected")

			if out.JSON {
				if err := out.PrintJSON(LinkStatus{
					Source:     string(source),
					Credential: identity.CredentialName,
					Workspace:  identity.WorkspaceName,
					Active:     false,
				}); err != nil {
					return fmt.Errorf("print link status json: %w", err)
				}

				return nil
			}

			out.Println()
			out.Print("Source:     %s\n", source)
			out.Print("Credential: %s\n", identity.CredentialName)
			out.Print("Workspace:  %s\n", identity.WorkspaceName)

			out.Println()
			out.Muted("Link: Not active")
			out.Muted("Run 'mush link' to start processing jobs.")

			return nil
		},
	}
}

// newUnlinkCmd creates the unlink command (graceful disconnect placeholder).
func newUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink",
		Short: "Gracefully disconnect from habitat",
		Long: `Gracefully disconnect from the current habitat.

Note: This is typically handled automatically via Ctrl+C when running 'mush link'.
This command is provided for programmatic disconnection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			out.Info("Links are disconnected via Ctrl+C when running 'mush link'")
			out.Muted("No active link to disconnect.")

			return nil
		},
	}
}
