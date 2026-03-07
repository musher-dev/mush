package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/update"
)

func newUpdateAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "__ua",
		Short:   "Internal background update agent",
		Long:    "Internal command used by mush to perform background update checks and staged applies.",
		Example: `  mush __ua`,
		Hidden:  true,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			return update.RunAgent(update.AgentConfig{
				CurrentVersion: buildinfo.Version,
				CheckInterval:  cfg.UpdateCheckInterval(),
				AutoApply:      cfg.UpdateAutoApply(),
			})
		},
	}

	return cmd
}
