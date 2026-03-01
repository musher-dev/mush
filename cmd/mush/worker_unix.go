//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
)

func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage the local worker runtime",
		Long: `Manage the local worker runtime that connects your machine to a habitat
and processes jobs from the Musher platform.

Use subcommands to start, check status, or stop the worker.`,
		Example: `  mush worker start
  mush worker start --habitat prod --queue jobs
  mush worker status
  mush worker stop`,
		Args: noArgs,
	}

	cmd.AddCommand(newWorkerStartCmd())
	cmd.AddCommand(newWorkerStatusCmd())
	cmd.AddCommand(newWorkerStopCmd())

	return cmd
}

func newWorkerStartCmd() *cobra.Command {
	var (
		dryRun       bool
		queue        string
		habitat      string
		harnessType  string
		bundleRef    string
		forceSidebar bool
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the worker and begin processing jobs",
		Long: `Start the worker, connecting your machine to a habitat and processing jobs.

Watch mode is the only supported surface. Mush runs an interactive terminal UI
that lets you watch job execution live.

The worker will:
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
Press Ctrl+S to toggle copy mode (Esc to return to live input).`,
		Example: `  mush worker start
  mush worker start --habitat prod --queue jobs
  mush worker start --harness claude
  mush worker start --bundle my-kit:0.1.0
  mush worker start --dry-run`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			logger := observability.FromContext(cmd.Context()).With(
				slog.String("component", "worker"),
				slog.String("event.type", "worker.start"),
			)

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
			var unavailable []string

			for _, h := range supportedHarnesses {
				info, ok := harness.Lookup(h)
				if !ok {
					continue
				}

				if !info.Available() {
					switch {
					case dryRun:
						out.Warning("%s CLI not found (dry-run mode, continuing)", h)
						out.Println()
					case len(supportedHarnesses) == 1:
						// Single harness mode: fail immediately.
						if h == "claude" {
							return clierrors.ClaudeNotFound()
						}

						return clierrors.HarnessNotAvailable(h)
					default:
						unavailable = append(unavailable, h)
					}
				}
			}

			if !dryRun && len(unavailable) > 0 && len(unavailable) < len(supportedHarnesses) {
				for _, h := range unavailable {
					out.Warning("%s CLI not found, disabling %s harness", h, h)
				}

				filtered := supportedHarnesses[:0]
				for _, h := range supportedHarnesses {
					if !slices.Contains(unavailable, h) {
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
				spin.Stop()
				return clierrors.AuthFailed(err)
			}

			spin.StopWithSuccess("Connected to " + apiURL)
			out.Print("Authenticated as: %s (Workspace: %s)\n", identity.CredentialName, identity.WorkspaceName)

			var runnerConfig *client.RunnerConfigResponse

			runnerConfig, err = c.GetRunnerConfig(cmd.Context())
			if err != nil {
				logger.Warn("runner config unavailable", slog.String("event.type", "worker.runner_config.unavailable"), slog.String("error", err.Error()))
				out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", err)
			}

			// Resolve habitat ID
			habitatID, err := resolveHabitatID(cmd.Context(), c, habitat, out)
			if err != nil {
				return err
			}

			queue, err := resolveQueue(cmd.Context(), c, habitatID, queue, out)
			if err != nil {
				return err
			}

			queueID := queue.ID
			bundleSummary := harness.BundleSummary{}

			// Install bundle assets if --bundle flag is set.
			if bundleRef != "" {
				var bundleErr error

				bundleSummary, bundleErr = resolveBundle(cmd.Context(), c, identity.WorkspaceID, bundleRef, supportedHarnesses, out)
				if bundleErr != nil {
					return bundleErr
				}
			}

			availability, err := c.GetQueueInstructionAvailability(cmd.Context(), queueID)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitNetwork, "Failed to check queue configuration", err).
					WithHint("Check your network connection or run 'mush doctor'")
			}

			if availability == nil || !availability.HasActiveInstruction {
				return clierrors.NoInstructionsForQueue(queue.Name, queue.Slug)
			}

			out.Print("Surface: watch\n")
			out.Print("Harnesses: %s\n", strings.Join(supportedHarnesses, ", "))
			out.Print("Queue ID: %s\n", queueID)

			if slices.Contains(supportedHarnesses, "claude") {
				mcpServers := harness.LoadedMCPServers(runnerConfig, time.Now())
				logger.Info(
					"MCP servers evaluated",
					slog.String("event.type", "mcp.specs.built"),
					slog.Int("mcp.server_count", len(mcpServers)),
					slog.Any("mcp.server_names", mcpServers),
				)

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
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
			defer stop()

			out.Println()

			err = runWatch(ctx, c, habitatID, queueID, supportedHarnesses, runnerConfig, &bundleSummary, forceSidebar)
			if err != nil {
				logger.Error("worker watch runtime failed", slog.String("event.type", "worker.error"), slog.String("error", err.Error()))
				return err
			}

			logger.Info("worker watch exited", slog.String("event.type", "worker.ready"))

			if cmd.Context().Err() == nil && ctx.Err() != nil {
				out.Println()
				out.Info("Received shutdown signal...")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Verify connection without claiming jobs")
	cmd.Flags().StringVar(&queue, "queue", "", "Filter jobs by queue slug or ID")
	cmd.Flags().StringVar(&habitat, "habitat", "", "Habitat slug or ID to connect to")
	cmd.Flags().StringVar(&harnessType, "harness", "", "Specific harness type: claude, bash (default: all)")
	cmd.Flags().StringVar(&bundleRef, "bundle", "", "Bundle slug[:version] to install before starting")
	cmd.Flags().BoolVar(&forceSidebar, "force-sidebar", false, "Skip terminal probe and force sidebar rendering")

	return cmd
}

func runWatch(
	ctx context.Context,
	c *client.Client,
	habitatID, queueID string,
	supportedHarnesses []string,
	runnerConfig *client.RunnerConfigResponse,
	bundleSummary *harness.BundleSummary,
	forceSidebar bool,
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
		TranscriptLines:    localCfg.HistoryScrollbackLines(),
		ForceSidebar:       forceSidebar,
		BundleName:         bundleSummary.Name,
		BundleVer:          bundleSummary.Version,
		BundleSummary:      *bundleSummary,
	}

	if err := harness.Run(ctx, cfg); err != nil {
		return clierrors.Wrap(clierrors.ExitExecution, "Watch harness failed", err)
	}

	return nil
}

func normalizeHarnessType(harnessType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(harnessType))

	if _, ok := harness.Lookup(normalized); ok {
		return normalized, nil
	}

	return "", clierrors.InvalidHarnessType(normalized, harness.RegisteredNames())
}

func defaultSupportedHarnesses() []string {
	return harness.AvailableNames()
}

// resolveBundle pulls and installs a bundle when the --bundle flag is set.
func resolveBundle(
	ctx context.Context,
	c *client.Client,
	workspaceKey string,
	bundleFlag string,
	supportedHarnesses []string,
	out *output.Writer,
) (harness.BundleSummary, error) {
	emptySummary := harness.BundleSummary{}
	logger := observability.FromContext(ctx).With(
		slog.String("component", "worker"),
		slog.String("event.type", "worker.bundle"),
	)

	ref, err := bundle.ParseRef(bundleFlag)
	if err != nil {
		return emptySummary, &clierrors.CLIError{
			Message: err.Error(),
			Hint:    "Use format: <slug> or <slug>:<version>",
			Code:    clierrors.ExitUsage,
		}
	}

	logger = logger.With(slog.String("bundle.slug", ref.Slug))

	// Determine harness type for asset mapping — use first supported harness.
	if len(supportedHarnesses) == 0 {
		return emptySummary, &clierrors.CLIError{
			Message: "No supported harnesses available for bundle install",
			Hint:    "Specify a harness with --harness or ensure at least one harness is available",
			Code:    clierrors.ExitUsage,
		}
	}

	harnessType := supportedHarnesses[0]

	mapper := mapperForHarness(harnessType)
	if mapper == nil {
		return emptySummary, &clierrors.CLIError{
			Message: fmt.Sprintf("No asset mapper for harness type: %s", harnessType),
			Hint:    "This harness type does not support bundle assets",
			Code:    clierrors.ExitUsage,
		}
	}

	// Pull the bundle.
	spin := out.Spinner(fmt.Sprintf("Pulling bundle %s", ref.Slug))
	spin.Start()

	resolved, cachePath, err := bundle.Pull(ctx, c, workspaceKey, ref.Slug, ref.Version, out)
	if err != nil {
		spin.StopWithFailure(fmt.Sprintf("Failed to pull bundle %s", ref.Slug))
		logger.Error("bundle pull failed", slog.String("event.type", "worker.bundle.error"), slog.String("error", err.Error()))

		return emptySummary, clierrors.Wrap(clierrors.ExitNetwork, "Failed to pull bundle", err).
			WithHint("Check your network connection and bundle slug")
	}

	spin.StopWithSuccess(fmt.Sprintf("Pulled bundle %s v%s", ref.Slug, resolved.Version))

	// Install assets into the working directory.
	workDir, err := os.Getwd()
	if err != nil {
		return emptySummary, clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
	}

	installedPaths, installErr := bundle.InstallFromCache(workDir, cachePath, &resolved.Manifest, mapper, true)
	if installErr != nil {
		var conflict *bundle.InstallConflictError
		if errors.As(installErr, &conflict) {
			logger.Warn("bundle install conflict", slog.String("event.type", "worker.bundle.conflict"), slog.String("error", installErr.Error()))
			return emptySummary, clierrors.InstallConflict(conflict.Path)
		}

		logger.Error("bundle install failed", slog.String("event.type", "worker.bundle.error"), slog.String("error", installErr.Error()))

		return emptySummary, clierrors.Wrap(clierrors.ExitGeneral, "Failed to install bundle assets", installErr)
	}

	for _, relPath := range installedPaths {
		out.Success("Installed: %s", relPath)
	}

	// Track the installation.
	trackErr := bundle.TrackInstall(workDir, &bundle.InstalledBundle{
		Slug:      ref.Slug,
		Version:   resolved.Version,
		Harness:   harnessType,
		Assets:    installedPaths,
		Timestamp: time.Now(),
	})
	if trackErr != nil {
		out.Warning("Failed to track installation: %v", trackErr)
	}

	out.Success("Bundle %s v%s installed (%d assets)", ref.Slug, resolved.Version, len(installedPaths))

	logger.Info(
		"bundle installed for worker",
		slog.String("event.type", "worker.bundle.installed"),
		slog.String("bundle.version", resolved.Version),
		slog.Int("bundle.asset_count", len(installedPaths)),
	)

	summary := harness.SummarizeBundleManifest(&resolved.Manifest)
	summary.Name = ref.Slug
	summary.Version = resolved.Version

	return summary, nil
}
