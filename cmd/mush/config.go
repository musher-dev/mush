package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and modify Mush configuration settings.`,
	}

	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())

	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			cfg := config.Load()
			settings := cfg.All()

			if out.JSON {
				return out.PrintJSON(settings)
			}

			if len(settings) == 0 {
				out.Muted("No configuration set.")
				out.Println()
				out.Println("Available settings:")
				out.Print("  api.url       Platform API URL (default: %s)\n", config.DefaultAPIURL)
				out.Print("  worker.poll   Poll interval in seconds (default: %d)\n", config.DefaultPollInterval)
				out.Print("  history.enabled   Enable PTY transcript capture (default: true)\n")
				out.Print("  history.dir       Transcript storage directory (default: ~/.config/mush/history)\n")
				out.Print("  history.lines     In-memory transcript lines per session (default: 10000)\n")
				out.Print("  history.retention Default prune window (default: 720h)\n")

				return nil
			}

			for key, value := range settings {
				out.Print("%s = %v\n", key, value)
			}

			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			key := args[0]
			cfg := config.Load()
			value := cfg.Get(key)

			if value == nil {
				out.Muted("%s is not set", key)
				return nil
			}

			out.Print("%s = %v\n", key, value)

			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			key, value := args[0], args[1]
			cfg := config.Load()

			if err := cfg.Set(key, value); err != nil {
				return clierrors.ConfigFailed("set config", err)
			}

			out.Success("Set %s = %s", key, value)

			return nil
		},
	}
}
