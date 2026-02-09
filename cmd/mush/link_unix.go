//go:build unix

package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/claude"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

func newLinkCmd() *cobra.Command {
	var (
		dryRun    bool
		queueID   string
		habitat   string
		agentType string
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
  4. Execute handlers locally using the appropriate agent (Claude, Bash)
  5. Report results back to the platform

Agent Types:
  --agent claude  Only handle Claude Code jobs
  --agent bash    Only handle Bash script jobs
  (default)       Handle all supported agent types

Press Ctrl+Q to exit the watch UI.

Examples:
  mush link                  # Watch mode, all agents
  mush link --agent claude   # Watch mode, Claude only
  mush link --agent bash     # Watch mode, Bash only
  mush link --habitat local  # Link to specific habitat by slug
  mush link --dry-run        # Verify connection without claiming jobs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			// Validate agent type if specified
			var supportedAgents []string
			if agentType != "" {
				agentType = strings.ToLower(agentType)
				if agentType != "claude" && agentType != "bash" {
					return clierrors.InvalidAgentType(agentType)
				}
				supportedAgents = []string{agentType}
			} else {
				// Default: support all agents
				supportedAgents = []string{"claude", "bash"}
			}

			// Check if required agents are available
			hasClaude := false
			hasBash := false
			claudeUnavailable := false
			for _, at := range supportedAgents {
				switch at {
				case "claude":
					hasClaude = true
					if !claude.Available() {
						switch {
						case dryRun:
							out.Warning("Claude Code CLI not found (dry-run mode, continuing)")
							out.Println()
						case len(supportedAgents) == 1:
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
				out.Warning("Claude Code CLI not found, disabling Claude agent (bash still enabled)")
				filtered := supportedAgents[:0]
				for _, at := range supportedAgents {
					if at != "claude" {
						filtered = append(filtered, at)
					}
				}
				supportedAgents = filtered
				out.Println()
			}

			// Get credentials and create client
			source, c, err := newAPIClient()
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
				return err
			}

			spin.StopWithSuccess("Connected to " + apiURL)
			out.Print("Authenticated as: %s (Workspace: %s)\n", identity.Email, identity.Workspace)

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
				return err
			}
			if availability == nil || !availability.HasActiveInstruction {
				return clierrors.NoInstructionsForQueue(queue.Name, queue.Slug)
			}

			out.Print("Surface: watch\n")
			out.Print("Agents: %s\n", strings.Join(supportedAgents, ", "))
			out.Print("Queue ID: %s\n", queueID)

			if dryRun {
				out.Println()
				out.Success("Dry run mode: connection verified, not claiming jobs")
				return nil
			}

			// Watch mode requires a terminal for the harness UI
			term := terminal.Detect()
			if !term.IsTTY {
				return &clierrors.CLIError{
					Message: "Watch mode requires a terminal (TTY)",
					Hint:    "Run this command directly in a terminal, not in a pipe or script",
					Code:    clierrors.ExitUsage,
				}
			}

			// Setup graceful shutdown
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			go func() {
				select {
				case <-sigCh:
					out.Println()
					out.Info("Received shutdown signal...")
					cancel()
				case <-ctx.Done():
					// Context canceled elsewhere (e.g. Ctrl+Q exit); avoid goroutine leak.
				}
			}()

			out.Println()
			return runWatchLink(ctx, c, habitatID, queueID, supportedAgents)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify connection without claiming jobs")
	cmd.Flags().StringVarP(&queueID, "queue-id", "q", "", "Filter jobs by queue ID")
	cmd.Flags().StringVar(&habitat, "habitat", "", "Habitat slug or ID to link to")
	cmd.Flags().StringVarP(&agentType, "agent", "a", "", "Specific agent type: claude, bash (default: all)")

	cmd.AddCommand(newLinkStatusCmd())
	return cmd
}

func runWatchLink(ctx context.Context, c *client.Client, habitatID, queueID string, supportedAgents []string) error {
	cfg := &harness.Config{
		Client:          c,
		HabitatID:       habitatID,
		QueueID:         queueID,
		SupportedAgents: supportedAgents,
	}
	return harness.Run(ctx, cfg)
}
