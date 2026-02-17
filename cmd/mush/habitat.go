package main

import (
	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
)

func newHabitatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "habitat",
		Short: "Manage habitats",
		Long: `Commands for listing and selecting habitats.

Habitats are execution contexts within your workspace where harnesses connect
and jobs are routed. You must select a habitat before linking to receive jobs.`,
	}

	cmd.AddCommand(newHabitatListCmd())

	return cmd
}

func newHabitatListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available habitats",
		Long:  `List all habitats available in your workspace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			// Get credentials and create client
			_, c, err := newAPIClient()
			if err != nil {
				return err
			}

			// Fetch habitats with spinner
			spin := out.Spinner("Fetching habitats")
			spin.Start()

			habitats, err := c.ListHabitats(cmd.Context())
			if err != nil {
				spin.StopWithFailure("Failed to fetch habitats")
				return err
			}

			spin.StopWithSuccess("Found habitats")

			if len(habitats) == 0 {
				return clierrors.NoHabitats()
			}

			if out.JSON {
				return out.PrintJSON(habitats)
			}

			out.Println()

			// Print header
			out.Print("%-20s %-30s %-10s %-10s\n", "SLUG", "NAME", "STATUS", "TYPE")
			out.Print("%-20s %-30s %-10s %-10s\n", "----", "----", "------", "----")

			// Print habitats
			for _, h := range habitats {
				// Truncate name if too long
				name := h.Name
				if len(name) > 28 {
					name = name[:25] + "..."
				}
				out.Print("%-18s %-30s %-10s %-10s\n", h.Slug, name, h.Status, h.HabitatType)
			}

			return nil
		},
	}
}
