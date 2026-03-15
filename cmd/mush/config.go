package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/paths"
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
		Long:  `Display all configuration settings and their current values. Shows available settings with defaults when none are set.`,
		Example: `  mush config list
  mush config list --json`,
		Args: noArgs,
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

				historyDir := "<user state dir>/mush/history"
				if resolved, err := paths.HistoryDir(); err == nil {
					historyDir = resolved
				}

				out.Print("  api.url       Platform API URL (default: %s)\n", config.DefaultAPIURL)
				out.Print("  worker.poll_interval   Poll interval (default: %s)\n", config.DefaultPollInterval)
				out.Print("  history.enabled   Enable PTY transcript capture (default: true)\n")
				out.Print("  history.dir       Transcript storage directory (default: %s)\n", historyDir)
				out.Print("  history.scrollback_lines  In-memory scrollback lines per session (default: 10000)\n")
				out.Print("  history.retention Default prune window (default: 720h)\n")

				return nil
			}

			flat := flattenSettings(settings)

			keys := make([]string, 0, len(flat))
			for key := range flat {
				keys = append(keys, key)
			}

			sort.Strings(keys)

			for _, key := range keys {
				value := flat[key]
				out.Print("%s = %v\n", key, value)
			}

			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Get a configuration value",
		Long:    `Retrieve and display the current value of a single configuration key.`,
		Example: `  mush config get api.url`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			key := args[0]
			cfg := config.Load()
			value := cfg.Get(key)

			if value == nil {
				out.Muted("%s is not set", key)
				return nil
			}

			out.Print("%s = %v\n", key, formatConfigValue(value))

			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "set <key> <value>",
		Short:   "Set a configuration value",
		Long:    `Set a configuration key to the given value. The value is persisted to the config file.`,
		Example: `  mush config set api.url https://api.example.com`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			key, value := args[0], args[1]
			cfg := config.Load()

			parsedValue, err := parseConfigValue(key, value)
			if err != nil {
				return clierrors.ConfigFailed("set config", err)
			}

			if err := cfg.Set(key, parsedValue); err != nil {
				return clierrors.ConfigFailed("set config", err)
			}

			out.Success("Set %s = %v", key, parsedValue)

			return nil
		},
	}
}

func parseConfigValue(key, value string) (interface{}, error) {
	if key == "keybindings" {
		return nil, errors.New("set individual keybindings via keybindings.<action>")
	}

	if strings.HasPrefix(key, "keybindings.") {
		action := strings.TrimPrefix(key, "keybindings.")
		if !config.IsKnownKeybindingAction(action) {
			return nil, clierrors.New(1, "unknown keybinding action: "+action)
		}

		parsed, err := config.ParseKeybindingValue(value)
		if err != nil {
			return nil, clierrors.Wrap(1, "parse keybinding", err)
		}

		return parsed, nil
	}

	return value, nil
}

func flattenSettings(settings map[string]interface{}) map[string]interface{} {
	flat := make(map[string]interface{})
	flattenInto(flat, "", settings)

	return flat
}

func flattenInto(dst map[string]interface{}, prefix string, value interface{}) {
	nested, ok := value.(map[string]interface{})
	if !ok {
		if prefix != "" {
			dst[prefix] = value
		}

		return
	}

	for key, child := range nested {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		flattenInto(dst, fullKey, child)
	}
}

func formatConfigValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, fmt.Sprint(item))
		}

		return items
	default:
		return value
	}
}
