//go:build unix

package main

import (
	"context"
	"fmt"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/claude"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/output"
)

func newLinkCmd() *cobra.Command {
	var (
		dryRun      bool
		queueID     string
		habitat     string
		harnessType string
	)

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link this machine to a habitat (watch mode)",
		Long: `Link your machine to a habitat and start processing jobs.

Watch mode is the only supported surface. Mush runs an interactive terminal UI
that lets you watch job execution live.

The link will:
  1. Connect to the Musher platform
  2. Register with the selected habitat
  3. Poll for available jobs
  4. Execute handlers locally using the appropriate harness (Claude, Bash)
  5. Report results back to the platform

Harness Types:
  --harness claude  Only handle Claude Code jobs
  --harness bash    Only handle Bash script jobs
  (default)         Handle all supported harness types

Press Ctrl+C once to interrupt Claude; press Ctrl+C again quickly to exit.
Press Ctrl+Q to exit the watch UI immediately.
Press Ctrl+S to toggle copy mode (Esc to return to live input).

Examples:
  mush link                    # Watch mode, all harnesses
  mush link --harness claude   # Watch mode, Claude only
  mush link --harness bash     # Watch mode, Bash only
  mush link --habitat local    # Link to specific habitat by slug
  mush link --dry-run          # Verify connection without claiming jobs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			// Validate harness type if specified.
			var supportedHarnesses []string

			if harnessType != "" {
				normalized, err := normalizeHarnessType(harnessType)
				if err != nil {
					return err
				}

				supportedHarnesses = []string{normalized}
			} else {
				supportedHarnesses = defaultSupportedHarnesses()
			}

			// Check if required harnesses are available.
			hasClaude := false
			hasBash := false
			claudeUnavailable := false

			for _, h := range supportedHarnesses {
				switch h {
				case "claude":
					hasClaude = true

					if !claude.Available() {
						switch {
						case dryRun:
							out.Warning("Claude Code CLI not found (dry-run mode, continuing)")
							out.Println()
						case len(supportedHarnesses) == 1:
							return clierrors.ClaudeNotFound()
						default:
							claudeUnavailable = true
						}
					}
				case "bash":
					hasBash = true
					// Bash availability is checked at execution time (exec.LookPath("bash")).
				}
			}

			if !dryRun && claudeUnavailable && hasClaude && hasBash {
				out.Warning("Claude Code CLI not found, disabling Claude harness (bash still enabled)")

				filtered := supportedHarnesses[:0]
				for _, h := range supportedHarnesses {
					if h != "claude" {
						filtered = append(filtered, h)
					}
				}

				supportedHarnesses = filtered

				out.Println()
			}

			// Get credentials and create client
			source, c, err := apiClientFactory()
			if err != nil {
				return err
			}

			out.Print("Using credentials from: %s\n", source)

			apiURL := c.BaseURL()

			// Validate connection with spinner
			spin := out.Spinner("Connecting to platform")
			spin.Start()

			identity, err := c.ValidateKey(cmd.Context())
			if err != nil {
				spin.StopWithFailure("Failed to authenticate")
				return fmt.Errorf("validate credentials: %w", err)
			}

			spin.StopWithSuccess("Connected to " + apiURL)
			out.Print("Authenticated as: %s (Workspace: %s)\n", identity.CredentialName, identity.WorkspaceName)

			var runnerConfig *client.RunnerConfigResponse

			runnerConfig, err = c.GetRunnerConfig(cmd.Context())
			if err != nil {
				out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", err)
			}

			// Resolve habitat ID
			habitatID, err := resolveHabitatID(cmd.Context(), c, habitat, out)
			if err != nil {
				return err
			}

			queue, err := resolveQueue(cmd.Context(), c, habitatID, queueID, out)
			if err != nil {
				return err
			}

			queueID = queue.ID

			availability, err := c.GetQueueInstructionAvailability(cmd.Context(), queueID)
			if err != nil {
				return fmt.Errorf("check queue instruction availability: %w", err)
			}

			if availability == nil || !availability.HasActiveInstruction {
				return clierrors.NoInstructionsForQueue(queue.Name, queue.Slug)
			}

			out.Print("Surface: watch\n")
			out.Print("Harnesses: %s\n", strings.Join(supportedHarnesses, ", "))
			out.Print("Queue ID: %s\n", queueID)

			if slices.Contains(supportedHarnesses, "claude") {
				mcpServers := harness.LoadedMCPServers(runnerConfig, time.Now())
				if len(mcpServers) == 0 {
					out.Print("MCP servers: none\n")
				} else {
					out.Print("MCP servers: %s\n", strings.Join(mcpServers, ", "))
				}
			}

			if dryRun {
				out.Println()
				out.Success("Dry run mode: connection verified, not claiming jobs")

				return nil
			}

			// Watch mode requires a terminal for the harness UI
			if !out.Terminal().IsTTY {
				return &clierrors.CLIError{
					Message: "Watch mode requires a terminal (TTY)",
					Hint:    "Run this command directly in a terminal, not in a pipe or script",
					Code:    clierrors.ExitUsage,
				}
			}

			// Setup graceful shutdown.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			out.Println()

			err = runWatchLink(ctx, c, habitatID, queueID, supportedHarnesses, runnerConfig)
			if err != nil {
				return err
			}

			if cmd.Context().Err() == nil && ctx.Err() != nil {
				out.Println()
				out.Info("Received shutdown signal...")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify connection without claiming jobs")
	cmd.Flags().StringVarP(&queueID, "queue-id", "q", "", "Filter jobs by queue ID")
	cmd.Flags().StringVar(&habitat, "habitat", "", "Habitat slug or ID to link to")
	cmd.Flags().StringVar(&harnessType, "harness", "", "Specific harness type: claude, bash (default: all)")

	cmd.AddCommand(newLinkStatusCmd())

	return cmd
}

func runWatchLink(
	ctx context.Context,
	c *client.Client,
	habitatID, queueID string,
	supportedHarnesses []string,
	runnerConfig *client.RunnerConfigResponse,
) error {
	localCfg := config.Load()
	cfg := &harness.Config{
		Client:             c,
		HabitatID:          habitatID,
		QueueID:            queueID,
		SupportedHarnesses: supportedHarnesses,
		RunnerConfig:       runnerConfig,
		TranscriptEnabled:  localCfg.HistoryEnabled(),
		TranscriptDir:      localCfg.HistoryDir(),
		TranscriptLines:    localCfg.HistoryLines(),
	}

	if err := harness.Run(ctx, cfg); err != nil {
		return fmt.Errorf("run watch harness: %w", err)
	}

	return nil
}

func normalizeHarnessType(harnessType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(harnessType))
	switch normalized {
	case "claude", "bash":
		return normalized, nil
	default:
		return "", clierrors.InvalidHarnessType(normalized)
	}
}

func defaultSupportedHarnesses() []string {
	return []string{"claude", "bash"}
}
