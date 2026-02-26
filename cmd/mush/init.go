package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/wizard"
)

func newInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Setup Mush for first use",
		Long: `Initialize Mush with a guided setup wizard.

The wizard will:
  1. Prompt for your API key
  2. Validate the connection
  3. Store credentials securely
  4. Show next steps

If credentials already exist, use --force to overwrite them.`,
		Example: `  mush init`,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			w := wizard.New(out, force)

			return w.Run(cmd.Context())
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing credentials without prompting")

	return cmd
}
