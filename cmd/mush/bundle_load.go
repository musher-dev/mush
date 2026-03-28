//go:build unix

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/tui/nav"
)

func newBundleLoadCmd() *cobra.Command {
	var (
		harnessType  string
		forceSidebar bool
		dirPath      string
		useSample    bool
		cacheOnly    bool
	)

	cmd := &cobra.Command{
		Use:   "load [<namespace/slug>[:<version>]]",
		Short: "Load a bundle into an ephemeral session",
		Long: `Pull a bundle and launch the TUI at the Ready screen where you can choose
to Run or Install. Use --no-tui to skip the TUI and launch the harness
directly (requires --harness).

Use --cache to download and cache a bundle without launching a session.
This is useful for pre-warming the bundle cache in CI or container builds.

Alternatively, load a bundle from a local directory with --dir or use the
built-in sample bundle with --sample for testing.`,
		Example: `  mush bundle load acme/my-kit
  mush bundle load acme/my-kit:0.1.0
  mush bundle load acme/my-kit --cache
  mush bundle load acme/my-kit --no-tui --harness claude
  mush bundle load --dir ./my-bundle --no-tui --harness claude
  mush bundle load --sample --no-tui --harness claude`,
		Args: func(cmd *cobra.Command, args []string) error {
			hasDir := cmd.Flags().Changed("dir") && dirPath != ""
			hasSample := cmd.Flags().Changed("sample") && useSample
			hasLocal := hasDir || hasSample

			if cmd.Flags().Changed("dir") && dirPath == "" {
				return clierrors.New(clierrors.ExitUsage, "--dir requires a non-empty directory path")
			}

			if hasLocal && len(args) > 0 {
				return clierrors.New(clierrors.ExitUsage, "Cannot specify both a bundle reference and --"+localFlagName(hasDir, hasSample))
			}

			if !hasLocal && len(args) != 1 {
				return clierrors.New(clierrors.ExitUsage, "Requires a bundle reference argument, --dir, or --sample")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			logger := observability.FromContext(cmd.Context()).With(
				slog.String("component", "bundle"),
				slog.String("event.type", "bundle.load.start"),
			)

			if !cacheOnly && !out.Terminal().IsTTY {
				return &clierrors.CLIError{
					Message: "Bundle load requires a terminal (TTY)",
					Hint:    "Run this command directly in a terminal, not in a pipe or script.\nUse --cache to download without launching a session",
					Code:    clierrors.ExitUsage,
				}
			}

			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			useTUI := shouldShowTUI(noTUI, out)

			if !cacheOnly && !useTUI && harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required in --no-tui mode",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			source, err := resolveBundleSource(cmd.Context(), out, logger, bundleSourceOptions{
				dirPath:   dirPath,
				useSample: useSample,
				refArg:    firstArg(args),
			})
			if err != nil {
				return err
			}
			defer source.Cleanup()

			if cacheOnly {
				out.Success("Cached %s/%s:%s", source.Ref.Namespace, source.Ref.Slug, source.Resolved.Version)
				logger.Info("bundle cached",
					slog.String("bundle.namespace", source.Ref.Namespace),
					slog.String("bundle.slug", source.Ref.Slug),
					slog.String("bundle.version", source.Resolved.Version),
				)

				return nil
			}

			return executeBundleLoad(cmd, out, logger, source, harnessType, forceSidebar, useTUI)
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to use (required with --no-tui)")
	cmd.Flags().BoolVar(&forceSidebar, "force-sidebar", false, "Skip terminal probe and force sidebar rendering")
	cmd.Flags().StringVar(&dirPath, "dir", "", "Load bundle from a local directory")
	cmd.Flags().BoolVar(&useSample, "sample", false, "Load the built-in sample bundle")
	cmd.Flags().BoolVar(&cacheOnly, "cache", false, "Download and cache the bundle without launching a session")
	cmd.MarkFlagsMutuallyExclusive("dir", "sample")

	return cmd
}

// executeBundleLoad handles the shared post-resolution logic for bundle load.
func executeBundleLoad(
	cmd *cobra.Command,
	out *output.Writer,
	logger *slog.Logger,
	source *bundleSourceResult,
	harnessType string,
	forceSidebar bool,
	useTUI bool,
) error {
	if useTUI {
		deps := buildTUIDeps()
		deps.InitialBundle = &nav.BundleSeed{
			Namespace: source.Ref.Namespace,
			Slug:      source.Ref.Slug,
			Version:   source.Resolved.Version,
		}

		result, navErr := nav.Run(cmd.Context(), deps)
		if navErr != nil {
			return clierrors.Wrap(clierrors.ExitGeneral, "Interactive TUI failed", navErr)
		}

		switch result.Action {
		case nav.ActionBundleLoad:
			return handleBundleLoadNavResult(cmd, out, result)
		case nav.ActionBundleInstall:
			return handleBundleInstallNavResult(cmd, out, result)
		default:
			return nil
		}
	}

	normalized, err := normalizeHarnessType(harnessType)
	if err != nil {
		return err
	}

	info, ok := harness.Lookup(normalized)
	if !ok || !info.Available() {
		return clierrors.HarnessNotAvailable(normalized)
	}

	mapper := mapperForHarness(normalized)
	if mapper == nil {
		return &clierrors.CLIError{
			Message: fmt.Sprintf("No asset mapper for harness type: %s", normalized),
			Hint:    "This harness type does not support bundle assets",
			Code:    clierrors.ExitUsage,
		}
	}

	projectDir, err := os.Getwd()
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
	}

	spec, ok := harness.GetProvider(normalized)
	if !ok {
		return clierrors.New(clierrors.ExitGeneral, fmt.Sprintf("No provider spec for harness: %s", normalized))
	}

	session, err := bundle.PrepareLoadSession(
		cmd.Context(), projectDir, &source.Resolved.Manifest, spec, mapper,
	)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prepare bundle load session", err).
			WithHint("Re-run the bundle flow or check the log file for details")
	}
	defer session.Cleanup()

	for _, w := range session.Warnings {
		out.Warning("%s", w)
	}

	if len(session.Prepared) > 0 {
		for _, relPath := range session.Prepared {
			out.Success("Prepared: %s", relPath)
		}

		logger.Info("bundle load assets prepared", slog.Int("asset_count", len(session.Prepared)))
	}

	out.Success("Bundle assets prepared in load directory")
	out.Print("Assets: %d loaded\n", len(source.Resolved.Manifest.Layers))
	out.Println()
	logger.Info("bundle load ready", slog.String("bundle.version", source.Resolved.Version), slog.Int("bundle.asset_count", len(source.Resolved.Manifest.Layers)))

	var runnerConfig *client.RunnerConfigResponse

	_, apiClient, _, apiErr := tryAPIClient()
	if apiErr == nil && apiClient != nil && apiClient.IsAuthenticated() {
		runnerConfig, err = apiClient.GetRunnerConfig(cmd.Context())
		if err != nil {
			out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	cfg := &harness.Config{
		SupportedHarnesses: []string{normalized},
		ForceSidebar:       forceSidebar,
		BundleLoadMode:     true,
		BundleName:         source.Ref.Slug,
		BundleVer:          source.Resolved.Version,
		BundleDir:          session.BundleDir,
		BundleWorkDir:      session.WorkingDir,
		BundleEnv:          session.Env,
		RunnerConfig:       runnerConfig,
		BundleSummary:      harness.SummarizeBundleManifest(&source.Resolved.Manifest),
	}

	if err := harness.Run(ctx, cfg); err != nil {
		logger.Error("bundle load runtime failed", slog.String("error", err.Error()))
		return clierrors.Wrap(clierrors.ExitExecution, "Bundle load failed", err)
	}

	logger.Info("bundle load exited")

	if cmd.Context().Err() == nil && ctx.Err() != nil {
		out.Println()
		out.Info("Received shutdown signal...")
	}

	return nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
}
