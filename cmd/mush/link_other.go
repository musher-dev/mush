//go:build !unix

package main

import (
	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func newLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link this machine to a habitat (watch mode)",
		Long: `Link starts Mush in watch mode (interactive terminal UI).

Watch mode is currently supported only on Unix-like systems.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &clierrors.CLIError{
				Message: "Watch mode is not supported on this operating system",
				Hint:    "Run Mush on a Unix-like OS (macOS/Linux) to use 'mush link'",
				Code:    clierrors.ExitUsage,
			}
		},
	}

	cmd.AddCommand(newLinkStatusCmd())
	return cmd
}
