//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage agent bundles",
		Long: `Pull versioned collections of agent assets from the Musher platform
and either load them ephemerally or install them into a harness's native
directory structure.`,
	}

	cmd.AddCommand(newBundleLoadCmd())
	cmd.AddCommand(newBundleInstallCmd())
	cmd.AddCommand(newBundleListCmd())
	cmd.AddCommand(newBundleInfoCmd())
	cmd.AddCommand(newBundleUninstallCmd())

	return cmd
}

func newBundleLoadCmd() *cobra.Command {
	var harnessType string

	cmd := &cobra.Command{
		Use:   "load <slug>[:<version>]",
		Short: "Load a bundle into an ephemeral session",
		Long: `Pull a bundle and launch a harness with the bundle's assets injected
into a temporary directory. The session is interactive — exit the harness
(Ctrl+Q) to clean up.`,
		Example: `  mush bundle load my-kit --harness claude
  mush bundle load my-kit:0.1.0 --harness claude`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			logger := observability.FromContext(cmd.Context()).With(
				slog.String("component", "bundle"),
				slog.String("event.type", "bundle.load.start"),
			)

			// Parse bundle reference.
			ref, err := bundle.ParseRef(args[0])
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use format: <slug> or <slug>:<version>",
					Code:    clierrors.ExitUsage,
				}
			}

			logger = logger.With(slog.String("bundle.slug", ref.Slug))

			// Validate harness type.
			if harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required for bundle load",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
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

			// Check for TTY.
			if !out.Terminal().IsTTY {
				return &clierrors.CLIError{
					Message: "Bundle load requires a terminal (TTY)",
					Hint:    "Run this command directly in a terminal, not in a pipe or script",
					Code:    clierrors.ExitUsage,
				}
			}

			// Authenticate (anonymous fallback for public bundles).
			source, c, wsKeyOverride, err := tryAPIClient()
			if err != nil {
				return err
			}

			var workspaceKey string
			if wsKeyOverride != "" {
				workspaceKey = wsKeyOverride

				out.Info("No credentials found; attempting public bundle access")
			} else {
				out.Print("Using credentials from: %s\n", source)

				workspaceKey, err = resolveWorkspaceKey(cmd.Context(), c, out)
				if err != nil {
					return err
				}
			}

			// Pull the bundle.
			resolved, cachePath, err := bundle.Pull(cmd.Context(), c, workspaceKey, ref.Slug, ref.Version, out)
			if err != nil {
				logger.Error("bundle load pull failed", slog.String("event.type", "bundle.load.error"), slog.String("error", err.Error()))

				if !c.IsAuthenticated() && isForbiddenError(err) {
					return &clierrors.CLIError{
						Message: fmt.Sprintf("Failed to pull bundle: %s", ref.Slug),
						Hint:    "This bundle may be private. Run 'mush auth login' to authenticate",
						Cause:   err,
						Code:    clierrors.ExitAuth,
					}
				}

				return clierrors.Wrap(clierrors.ExitNetwork, "Failed to pull bundle", err).
					WithHint("Check your network connection and bundle slug")
			}

			// Get the mapper for this harness type.
			mapper := mapperForHarness(normalized)
			if mapper == nil {
				return &clierrors.CLIError{
					Message: fmt.Sprintf("No asset mapper for harness type: %s", normalized),
					Hint:    "This harness type does not support bundle assets",
					Code:    clierrors.ExitUsage,
				}
			}

			// Prepare load directory.
			tmpDir, cleanup, err := mapper.PrepareLoad(cmd.Context(), cachePath, &resolved.Manifest)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prepare load directory", err)
			}

			defer cleanup()

			// Inject discoverable assets (agents, skills) into the project directory.
			// For add_dir mode harnesses these are excluded from the temp dir
			// (via PrepareLoad) to avoid duplication, since the harness discovers
			// them from both CWD and --add-dir.
			projectDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			injected, assetWarnings, assetCleanup, err := bundle.InjectAssetsForLoad(
				projectDir, cachePath, &resolved.Manifest, mapper,
			)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to inject assets for load", err)
			}

			defer assetCleanup()

			for _, w := range assetWarnings {
				out.Warning("%s", w)
			}

			if len(injected) > 0 {
				for _, relPath := range injected {
					out.Success("Injected: %s", relPath)
				}

				logger.Info(
					"assets injected into project dir",
					slog.String("event.type", "bundle.load.assets_injected"),
					slog.Int("asset_count", len(injected)),
				)
			}

			// Inject tool configs into project dir for harnesses that read
			// tool config from CWD only (no --mcp-config flag).
			spec, _ := harness.GetProvider(normalized)
			if spec != nil && (spec.CLI == nil || spec.CLI.MCPConfig == "") {
				toolInjected, toolCleanup, toolErr := bundle.InjectToolConfigsForLoad(
					projectDir, cachePath, &resolved.Manifest, mapper,
				)
				if toolErr != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to inject tool configs for load", toolErr)
				}

				defer toolCleanup()

				for _, relPath := range toolInjected {
					out.Success("Injected: %s", relPath)
				}
			}

			out.Success("Bundle assets prepared in load directory")
			out.Print("Assets: %d loaded\n", len(resolved.Manifest.Layers))
			out.Println()
			logger.Info(
				"bundle load ready",
				slog.String("event.type", "bundle.load.ready"),
				slog.String("bundle.version", resolved.Version),
				slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)),
			)

			var runnerConfig *client.RunnerConfigResponse

			runnerConfig, err = c.GetRunnerConfig(cmd.Context())
			if err != nil {
				out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", err)
			}

			// Setup graceful shutdown.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
			defer stop()

			// Run TUI in load mode.
			cfg := &harness.Config{
				SupportedHarnesses: []string{normalized},
				BundleLoadMode:     true,
				BundleName:         ref.Slug,
				BundleVer:          resolved.Version,
				BundleDir:          tmpDir,
				RunnerConfig:       runnerConfig,
				BundleSummary:      harness.SummarizeBundleManifest(&resolved.Manifest),
			}

			if err := harness.Run(ctx, cfg); err != nil {
				logger.Error("bundle load runtime failed", slog.String("event.type", "bundle.load.error"), slog.String("error", err.Error()))
				return clierrors.Wrap(clierrors.ExitExecution, "Bundle load failed", err)
			}

			logger.Info("bundle load exited", slog.String("event.type", "bundle.load.exit"))

			if cmd.Context().Err() == nil && ctx.Err() != nil {
				out.Println()
				out.Info("Received shutdown signal...")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to use (required)")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

func newBundleInstallCmd() *cobra.Command {
	var (
		harnessType string
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "install <slug>[:<version>]",
		Short: "Install bundle assets into the current project",
		Long: `Pull a bundle and install its assets into the harness's native directory
structure in the current project directory.`,
		Example: `  mush bundle install my-kit --harness claude
  mush bundle install my-kit:0.1.0 --harness claude --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			logger := observability.FromContext(cmd.Context()).With(
				slog.String("component", "bundle"),
				slog.String("event.type", "bundle.install.start"),
			)

			// Parse bundle reference.
			ref, err := bundle.ParseRef(args[0])
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use format: <slug> or <slug>:<version>",
					Code:    clierrors.ExitUsage,
				}
			}

			logger = logger.With(slog.String("bundle.slug", ref.Slug))

			// Validate harness type.
			if harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required for bundle install",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			normalized, err := normalizeHarnessType(harnessType)
			if err != nil {
				return err
			}

			// Authenticate (anonymous fallback for public bundles).
			source, c, wsKeyOverride, err := tryAPIClient()
			if err != nil {
				return err
			}

			var workspaceKey string
			if wsKeyOverride != "" {
				workspaceKey = wsKeyOverride

				out.Info("No credentials found; attempting public bundle access")
			} else {
				out.Print("Using credentials from: %s\n", source)

				workspaceKey, err = resolveWorkspaceKey(cmd.Context(), c, out)
				if err != nil {
					return err
				}
			}

			// Pull the bundle.
			resolved, cachePath, err := bundle.Pull(cmd.Context(), c, workspaceKey, ref.Slug, ref.Version, out)
			if err != nil {
				logger.Error("bundle install pull failed", slog.String("event.type", "bundle.install.error"), slog.String("error", err.Error()))

				if !c.IsAuthenticated() && isForbiddenError(err) {
					return &clierrors.CLIError{
						Message: fmt.Sprintf("Failed to pull bundle: %s", ref.Slug),
						Hint:    "This bundle may be private. Run 'mush auth login' to authenticate",
						Cause:   err,
						Code:    clierrors.ExitAuth,
					}
				}

				return clierrors.Wrap(clierrors.ExitNetwork, "Failed to pull bundle", err).
					WithHint("Check your network connection and bundle slug")
			}

			// Get the mapper for this harness type.
			mapper := mapperForHarness(normalized)
			if mapper == nil {
				return &clierrors.CLIError{
					Message: fmt.Sprintf("No asset mapper for harness type: %s", normalized),
					Hint:    "This harness type does not support bundle assets",
					Code:    clierrors.ExitUsage,
				}
			}

			// Install assets.
			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installedPaths, installErr := bundle.InstallFromCache(workDir, cachePath, &resolved.Manifest, mapper, force)
			if installErr != nil {
				var conflict *bundle.InstallConflictError
				if errors.As(installErr, &conflict) {
					logger.Warn("bundle install conflict", slog.String("event.type", "bundle.install.conflict"), slog.String("error", installErr.Error()))
					return clierrors.InstallConflict(conflict.Path)
				}

				logger.Error("bundle install failed", slog.String("event.type", "bundle.install.error"), slog.String("error", installErr.Error()))

				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to install bundle assets", installErr)
			}

			for _, relPath := range installedPaths {
				out.Success("Installed: %s", relPath)
			}

			// Track the installation.
			trackErr := bundle.TrackInstall(workDir, &bundle.InstalledBundle{
				Slug:      ref.Slug,
				Version:   resolved.Version,
				Harness:   normalized,
				Assets:    installedPaths,
				Timestamp: time.Now(),
			})
			if trackErr != nil {
				out.Warning("Failed to track installation: %v", trackErr)
			}

			out.Println()
			out.Success("Installed %d assets from %s v%s", len(resolved.Manifest.Layers), ref.Slug, resolved.Version)
			logger.Info(
				"bundle install completed",
				slog.String("event.type", "bundle.install.complete"),
				slog.String("bundle.version", resolved.Version),
				slog.Int("bundle.asset_count", len(installedPaths)),
			)

			return nil
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to install for (required)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing files")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

func newBundleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local bundle cache and installed bundles",
		Long: `Show all bundles stored in the local cache and any bundles installed in the
current project directory.`,
		Example: `  mush bundle list`,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			cached, err := bundle.ListCached()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list cached bundles", err)
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installed, err := bundle.LoadInstalled(workDir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load installed bundles", err)
			}

			out.Println("Cached bundles:")

			if len(cached) == 0 {
				out.Print("  (none)\n")
			} else {
				for _, c := range cached {
					out.Print("  %s/%s:%s (%d assets)\n", c.Workspace, c.Slug, c.Version, c.AssetCount)
				}
			}

			out.Println()
			out.Println("Installed bundles in current project:")

			if len(installed) == 0 {
				out.Print("  (none)\n")
			} else {
				sort.Slice(installed, func(i, j int) bool {
					if installed[i].Slug != installed[j].Slug {
						return installed[i].Slug < installed[j].Slug
					}

					return installed[i].Harness < installed[j].Harness
				})

				for _, item := range installed {
					out.Print("  %s:%s [%s] (%d assets)\n", item.Slug, item.Version, item.Harness, len(item.Assets))
				}
			}

			return nil
		},
	}
}

func newBundleInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <slug>",
		Short: "Show local details for a bundle slug",
		Long: `Show cached versions and installation status for a specific bundle slug
in the current project directory.`,
		Example: `  mush bundle info my-agent-kit`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			slug := strings.TrimSpace(args[0])

			if slug == "" {
				return &clierrors.CLIError{
					Message: "Bundle slug is required",
					Hint:    "Use: mush bundle info <slug>",
					Code:    clierrors.ExitUsage,
				}
			}

			cached, err := bundle.ListCached()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list cached bundles", err)
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installed, err := bundle.LoadInstalled(workDir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load installed bundles", err)
			}

			var cachedMatches []bundle.CachedBundle

			for _, c := range cached {
				if c.Slug == slug {
					cachedMatches = append(cachedMatches, c)
				}
			}

			var installedMatches []bundle.InstalledBundle

			for _, item := range installed {
				if item.Slug == slug {
					installedMatches = append(installedMatches, item)
				}
			}

			if len(cachedMatches) == 0 && len(installedMatches) == 0 {
				return clierrors.BundleNotFound(slug)
			}

			out.Print("Bundle: %s\n\n", slug)

			out.Println("Cached versions:")

			if len(cachedMatches) == 0 {
				out.Print("  (none)\n")
			} else {
				for _, c := range cachedMatches {
					out.Print("  %s/%s:%s (%d assets)\n", c.Workspace, c.Slug, c.Version, c.AssetCount)
				}
			}

			out.Println()
			out.Println("Installed in current project:")

			if len(installedMatches) == 0 {
				out.Print("  (none)\n")
			} else {
				for _, item := range installedMatches {
					out.Print("  %s [%s] (%d assets)\n", item.Version, item.Harness, len(item.Assets))
				}
			}

			return nil
		},
	}
}

func newBundleUninstallCmd() *cobra.Command {
	var (
		harnessType string
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "uninstall <slug> --harness <type>",
		Short: "Remove installed bundle assets from the current project",
		Long: `Remove previously installed bundle assets from the current project directory.

Lists the files that will be removed and prompts for confirmation unless
--force is passed.`,
		Example: `  mush bundle uninstall my-kit --harness claude
  mush bundle uninstall my-kit --harness claude --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			slug := strings.TrimSpace(args[0])

			if harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required for bundle uninstall",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			normalized, err := normalizeHarnessType(harnessType)
			if err != nil {
				return err
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			// Preview what will be removed.
			entry, err := bundle.FindInstalled(workDir, slug, normalized)
			if err != nil {
				if errors.Is(err, bundle.ErrNotInstalled) {
					return clierrors.BundleNotFound(slug)
				}

				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read installed bundles", err)
			}

			out.Println("The following files will be removed:")

			for _, relPath := range entry.Assets {
				out.Print("  %s\n", relPath)
			}

			out.Println()

			// Require confirmation.
			if !force {
				if out.NoInput {
					return clierrors.New(clierrors.ExitUsage, "Cannot confirm uninstall in non-interactive mode").
						WithHint("Use --force to skip confirmation")
				}

				prompter := prompt.New(out)

				confirmed, promptErr := prompter.Confirm(
					fmt.Sprintf("Uninstall %s (%s)? This will remove %d file(s)", slug, normalized, len(entry.Assets)),
					false,
				)
				if promptErr != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read confirmation", promptErr)
				}

				if !confirmed {
					out.Info("Uninstall canceled")
					return nil
				}
			}

			removed, err := bundle.Uninstall(workDir, slug, normalized)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to uninstall bundle assets", err)
			}

			for _, relPath := range removed {
				out.Success("Removed: %s", relPath)
			}

			out.Println()
			out.Success("Uninstalled %s for harness %s", slug, normalized)

			return nil
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to uninstall from (required)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

// mapperForHarness returns the AssetMapper for a given harness type.
func mapperForHarness(harnessType string) bundle.AssetMapper {
	spec, ok := harness.GetProvider(harnessType)
	if !ok || !harness.HasAssetMapping(harnessType) {
		return nil
	}

	return bundle.NewProviderMapper(spec)
}

// joinNames joins a slice of strings with ", ".
func joinNames(names []string) string {
	result := ""

	for i, n := range names {
		if i > 0 {
			result += ", "
		}

		result += n
	}

	return result
}

func resolveWorkspaceKey(ctx context.Context, c *client.Client, out *output.Writer) (string, error) {
	identity, err := c.ValidateKey(ctx)
	if err != nil {
		return "", clierrors.AuthFailed(err)
	}

	if identity.WorkspaceID == "" {
		out.Warning("Workspace ID not present in identity; using local cache workspace key 'default'")
		return "default", nil
	}

	return identity.WorkspaceID, nil
}

// isForbiddenError returns true if the error chain contains an HTTP 403 status,
// indicating the resource requires authentication.
func isForbiddenError(err error) bool {
	return strings.Contains(err.Error(), "status 403")
}
