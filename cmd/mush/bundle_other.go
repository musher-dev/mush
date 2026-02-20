//go:build !unix

package main

import (
	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func newBundleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bundle",
		Short: "Manage agent bundles",
		Long:  `Bundle commands are currently supported only on Unix-like systems.`,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &clierrors.CLIError{
				Message: "Bundle commands are not supported on this operating system",
				Hint:    "Run Mush on a Unix-like OS (macOS/Linux) to use bundle commands",
				Code:    clierrors.ExitUsage,
			}
		},
	}
}
