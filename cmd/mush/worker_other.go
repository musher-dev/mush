//go:build !unix

package main

import (
	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage the local worker runtime",
		Long: `Manage the local worker runtime that connects your machine to a habitat
and processes jobs from the Musher platform.

Watch mode is currently supported only on Unix-like systems.`,
		Example: `  mush worker start
  mush worker start --habitat prod --queue jobs
  mush worker start --dry-run`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &clierrors.CLIError{
				Message: "Watch mode is not supported on this operating system",
				Hint:    "Run Mush on a Unix-like OS (macOS/Linux) to use 'mush worker start'",
				Code:    clierrors.ExitUsage,
			}
		},
	}

	cmd.AddCommand(newWorkerStatusCmd())
	cmd.AddCommand(newWorkerStopCmd())

	return cmd
}
