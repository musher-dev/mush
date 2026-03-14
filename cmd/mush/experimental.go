package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/output"
)

func newExperimentalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "experimental",
		Short: "Experimental features (requires MUSH_EXPERIMENTAL=1)",
		Long: `Access experimental features that are under active development.
Enable via MUSH_EXPERIMENTAL=1 environment variable or 'mush config set experimental true'.`,
		Example: `  MUSH_EXPERIMENTAL=1 mush experimental`,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := output.FromContext(cmd.Context())
			out.Print("No experimental features are currently available.\n")

			return nil
		},
	}

	return cmd
}
