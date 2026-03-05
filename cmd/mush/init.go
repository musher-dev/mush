package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/wizard"
)

func newInitCmd() *cobra.Command {
	var (
		force   bool
		apiKey  string
		habitat string
	)

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

			w := wizard.New(out, force, apiKey, habitat)

			return w.Run(cmd.Context())
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing credentials without prompting")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key to use for non-interactive initialization")
	cmd.Flags().StringVar(&habitat, "habitat", "", "Habitat slug or ID to select during initialization")

	return cmd
}
