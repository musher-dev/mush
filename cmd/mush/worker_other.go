//go:build !unix

package main

import (
	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func unsupportedWatchModeError() error {
	return &clierrors.CLIError{
		Message: "Watch mode is not supported on this operating system",
		Hint:    "Run Mush on a Unix-like OS (macOS/Linux) to use 'mush worker start'",
		Code:    clierrors.ExitUsage,
	}
}

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
			return unsupportedWatchModeError()
		},
	}

	cmd.AddCommand(newWorkerStartCmd())
	cmd.AddCommand(newWorkerStatusCmd())
	cmd.AddCommand(newWorkerStopCmd())

	return cmd
}

func newWorkerStartCmd() *cobra.Command {
	var (
		dryRun      bool
		queue       string
		habitat     string
		harnessType string
		bundleRef   string
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the worker and begin processing jobs",
		Long: `Start the worker, connecting your machine to a habitat and processing jobs.

Watch mode is currently supported only on Unix-like systems.`,
		Example: `  mush worker start
  mush worker start --habitat prod --queue jobs
  mush worker start --harness claude
  mush worker start --bundle my-kit:0.1.0
  mush worker start --dry-run`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return unsupportedWatchModeError()
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify connection without claiming jobs")
	cmd.Flags().StringVar(&queue, "queue", "", "Filter jobs by queue slug or ID")
	cmd.Flags().StringVar(&habitat, "habitat", "", "Habitat slug or ID to connect to")
	cmd.Flags().StringVar(&harnessType, "harness", "", "Specific harness type: claude, bash (default: all)")
	cmd.Flags().StringVar(&bundleRef, "bundle", "", "Bundle slug[:version] to install before starting")

	return cmd
}
