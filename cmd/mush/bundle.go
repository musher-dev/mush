//go:build unix

package main

import (
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
	"github.com/musher-dev/mush/internal/tui/nav"
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
	var (
		harnessType  string
		forceSidebar bool
		dirPath      string
		useSample    bool
	)

	cmd := &cobra.Command{
		Use:   "load [<namespace/slug>[:<version>]]",
		Short: "Load a bundle into an ephemeral session",
		Long: `Pull a bundle and launch the TUI at the Ready screen where you can choose
to Run or Install. Use --no-tui to skip the TUI and launch the harness
directly (requires --harness).

Alternatively, load a bundle from a local directory with --dir or use the
built-in sample bundle with --sample for testing.`,
		Example: `  mush bundle load acme/my-kit
  mush bundle load acme/my-kit:0.1.0
  mush bundle load acme/my-kit --no-tui --harness claude
  mush bundle load --dir ./my-bundle --no-tui --harness claude
  mush bundle load --sample --no-tui --harness claude`,
		Args: func(cmd *cobra.Command, args []string) error {
			hasDir := cmd.Flags().Changed("dir") && dirPath != ""
			hasSample := cmd.Flags().Changed("sample") && useSample
			hasLocal := hasDir || hasSample

			// Reject explicitly empty --dir value.
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

			// Check for TTY before anything else — it's the most fundamental requirement.
			if !out.Terminal().IsTTY {
				return &clierrors.CLIError{
					Message: "Bundle load requires a terminal (TTY)",
					Hint:    "Run this command directly in a terminal, not in a pipe or script",
					Code:    clierrors.ExitUsage,
				}
			}

			// Determine TUI vs direct mode.
			noTUI, _ := cmd.Root().PersistentFlags().GetBool("no-tui")
			useTUI := shouldShowTUI(noTUI, out)

			// In direct mode, --harness is required.
			if !useTUI && harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required in --no-tui mode",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			// Local source: --dir
			if dirPath != "" {
				resolved, cachePath, cleanup, err := bundle.LoadFromDir(dirPath)
				if err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load bundle from directory", err)
				}

				defer cleanup()

				logger.Info("bundle loaded from local directory",
					slog.String("event.type", "bundle.load.local"),
					slog.String("bundle.dir", dirPath),
					slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)),
				)

				ref := bundle.Ref{Namespace: resolved.Namespace, Slug: resolved.Slug}

				return executeBundleLoad(cmd, out, logger, resolved, cachePath, ref, harnessType, forceSidebar, useTUI)
			}

			// Local source: --sample
			if useSample {
				resolved, cachePath, cleanup, err := bundle.ExtractSampleBundle()
				if err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to extract sample bundle", err)
				}

				defer cleanup()

				logger.Info("sample bundle extracted",
					slog.String("event.type", "bundle.load.sample"),
					slog.Int("bundle.asset_count", len(resolved.Manifest.Layers)),
				)

				ref := bundle.Ref{Namespace: resolved.Namespace, Slug: resolved.Slug}

				return executeBundleLoad(cmd, out, logger, resolved, cachePath, ref, harnessType, forceSidebar, useTUI)
			}

			// Remote source: parse bundle reference and pull from API.
			ref, err := bundle.ParseRef(args[0])
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use format: namespace/slug or namespace/slug:version",
					Code:    clierrors.ExitUsage,
				}
			}

			logger = logger.With(slog.String("bundle.slug", ref.Slug), slog.String("bundle.namespace", ref.Namespace))

			// Authenticate (anonymous fallback for public bundles).
			source, c, _, err := tryAPIClient()
			if err != nil {
				return err
			}

			if source != "" {
				out.Print("Using credentials from: %s\n", source)
			} else {
				out.Info("No credentials found; attempting public bundle access")
			}

			// Pull the bundle.
			resolved, cachePath, err := bundle.Pull(cmd.Context(), c, ref.Namespace, ref.Slug, ref.Version, out)
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
					WithHint("Check your network connection and bundle reference")
			}

			return executeBundleLoad(cmd, out, logger, resolved, cachePath, ref, harnessType, forceSidebar, useTUI)
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to use (required with --no-tui)")
	cmd.Flags().BoolVar(&forceSidebar, "force-sidebar", false, "Skip terminal probe and force sidebar rendering")
	cmd.Flags().StringVar(&dirPath, "dir", "", "Load bundle from a local directory")
	cmd.Flags().BoolVar(&useSample, "sample", false, "Load the built-in sample bundle")
	cmd.MarkFlagsMutuallyExclusive("dir", "sample")

	return cmd
}

// executeBundleLoad handles the shared post-resolution logic for bundle load,
// used by all three source modes (API, --dir, --sample).
func executeBundleLoad(
	cmd *cobra.Command,
	out *output.Writer,
	logger *slog.Logger,
	resolved *client.BundleResolveResponse,
	cachePath string,
	ref bundle.Ref,
	harnessType string,
	forceSidebar bool,
	useTUI bool,
) error {
	// TUI path: launch nav at the bundle action screen.
	if useTUI {
		deps := buildTUIDeps()
		deps.InitialBundle = &nav.BundleSeed{
			Namespace: ref.Namespace,
			Slug:      ref.Slug,
			Version:   resolved.Version,
			CachePath: cachePath,
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

	// Direct (--no-tui) path: validate harness and launch directly.
	normalized, err := normalizeHarnessType(harnessType)
	if err != nil {
		return err
	}

	info, ok := harness.Lookup(normalized)
	if !ok || !info.Available() {
		return clierrors.HarnessNotAvailable(normalized)
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

	projectDir, err := os.Getwd()
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
	}

	spec, _ := harness.GetProvider(normalized)

	session, err := bundle.PrepareLoadSession(
		cmd.Context(), projectDir, cachePath, &resolved.Manifest, spec, mapper,
	)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prepare bundle load session", err)
	}

	defer session.Cleanup()

	for _, w := range session.Warnings {
		out.Warning("%s", w)
	}

	if len(session.Prepared) > 0 {
		for _, relPath := range session.Prepared {
			out.Success("Prepared: %s", relPath)
		}

		logger.Info(
			"bundle load assets prepared",
			slog.String("event.type", "bundle.load.assets_prepared"),
			slog.Int("asset_count", len(session.Prepared)),
		)
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

	_, c, _, apiErr := tryAPIClient()
	if apiErr == nil && c != nil {
		runnerConfig, err = c.GetRunnerConfig(cmd.Context())
		if err != nil {
			out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", err)
		}
	}

	// Setup graceful shutdown.
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	// Run TUI in load mode.
	cfg := &harness.Config{
		SupportedHarnesses: []string{normalized},
		ForceSidebar:       forceSidebar,
		BundleLoadMode:     true,
		BundleName:         ref.Slug,
		BundleVer:          resolved.Version,
		BundleDir:          session.BundleDir,
		BundleWorkDir:      session.WorkingDir,
		BundleEnv:          session.Env,
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
}

// localFlagName returns the flag name for error messages.
func localFlagName(hasDir, hasSample bool) string {
	if hasDir {
		return "dir"
	}

	if hasSample {
		return "sample"
	}

	return "local"
}

func newBundleInstallCmd() *cobra.Command {
	var (
		harnessType string
		force       bool
		dirPath     string
	)

	cmd := &cobra.Command{
		Use:   "install [<namespace/slug>[:<version>]]",
		Short: "Install bundle assets into the current project",
		Long: `Pull a bundle and install its assets into the harness's native directory
structure in the current project directory.

Alternatively, install from a local directory with --dir.`,
		Example: `  mush bundle install acme/my-kit --harness claude
  mush bundle install acme/my-kit:0.1.0 --harness claude --force
  mush bundle install --dir ./my-bundle --harness claude`,
		Args: func(cmd *cobra.Command, args []string) error {
			hasDir := cmd.Flags().Changed("dir") && dirPath != ""

			// Reject explicitly empty --dir value.
			if cmd.Flags().Changed("dir") && dirPath == "" {
				return clierrors.New(clierrors.ExitUsage, "--dir requires a non-empty directory path")
			}

			if hasDir && len(args) > 0 {
				return clierrors.New(clierrors.ExitUsage, "Cannot specify both a bundle reference and --dir")
			}

			if !hasDir && len(args) != 1 {
				return clierrors.New(clierrors.ExitUsage, "Requires a bundle reference argument or --dir")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			logger := observability.FromContext(cmd.Context()).With(
				slog.String("component", "bundle"),
				slog.String("event.type", "bundle.install.start"),
			)

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

			var (
				resolved  *client.BundleResolveResponse
				cachePath string
				ref       bundle.Ref
				cleanup   func()
			)

			if dirPath != "" {
				// Local source: --dir
				resolved, cachePath, cleanup, err = bundle.LoadFromDir(dirPath)
				if err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load bundle from directory", err)
				}

				defer cleanup()

				ref = bundle.Ref{Namespace: resolved.Namespace, Slug: resolved.Slug}

				logger.Info("bundle loaded from local directory",
					slog.String("event.type", "bundle.install.local"),
					slog.String("bundle.dir", dirPath),
				)
			} else {
				// Remote source: parse bundle reference and pull from API.
				ref, err = bundle.ParseRef(args[0])
				if err != nil {
					return &clierrors.CLIError{
						Message: err.Error(),
						Hint:    "Use format: namespace/slug or namespace/slug:version",
						Code:    clierrors.ExitUsage,
					}
				}

				logger = logger.With(slog.String("bundle.slug", ref.Slug), slog.String("bundle.namespace", ref.Namespace))

				// Authenticate (anonymous fallback for public bundles).
				source, c, _, apiErr := tryAPIClient()
				if apiErr != nil {
					return apiErr
				}

				if source != "" {
					out.Print("Using credentials from: %s\n", source)
				} else {
					out.Info("No credentials found; attempting public bundle access")
				}

				// Pull the bundle.
				resolved, cachePath, err = bundle.Pull(cmd.Context(), c, ref.Namespace, ref.Slug, ref.Version, out)
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
						WithHint("Check your network connection and bundle reference")
				}
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
				Namespace: ref.Namespace,
				Slug:      ref.Slug,
				Ref:       ref.Namespace + "/" + ref.Slug,
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
	cmd.Flags().StringVar(&dirPath, "dir", "", "Install bundle from a local directory")
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
					out.Print("  %s/%s:%s (%d assets)\n", c.Namespace, c.Slug, c.Version, c.AssetCount)
				}
			}

			out.Println()
			out.Println("Installed bundles in current project:")

			if len(installed) == 0 {
				out.Print("  (none)\n")
			} else {
				sort.Slice(installed, func(i, j int) bool {
					if installed[i].Ref != installed[j].Ref {
						return installed[i].Ref < installed[j].Ref
					}

					return installed[i].Harness < installed[j].Harness
				})

				for i := range installed {
					out.Print("  %s:%s [%s] (%d assets)\n", installed[i].Ref, installed[i].Version, installed[i].Harness, len(installed[i].Assets))
				}
			}

			return nil
		},
	}
}

func newBundleInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <namespace/slug>[:<version>]",
		Short: "Show local details for a bundle reference",
		Long: `Show cached versions and installation status for a specific bundle reference
in the current project directory.`,
		Example: `  mush bundle info acme/my-agent-kit
  mush bundle info acme/my-agent-kit:1.0.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			ref, err := bundle.ParseRef(strings.TrimSpace(args[0]))
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use: mush bundle info <namespace/slug>[:<version>]",
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

			for i := range cached {
				if cached[i].Namespace != ref.Namespace || cached[i].Slug != ref.Slug {
					continue
				}

				if ref.Version != "" && cached[i].Version != ref.Version {
					continue
				}

				cachedMatches = append(cachedMatches, cached[i])
			}

			var installedMatches []bundle.InstalledBundle

			for i := range installed {
				if installed[i].Namespace != ref.Namespace || installed[i].Slug != ref.Slug {
					continue
				}

				if ref.Version != "" && installed[i].Version != ref.Version {
					continue
				}

				installedMatches = append(installedMatches, installed[i])
			}

			if len(cachedMatches) == 0 && len(installedMatches) == 0 {
				return clierrors.BundleNotFound(ref.String())
			}

			out.Print("Bundle: %s\n\n", ref.String())

			out.Println("Cached versions:")

			if len(cachedMatches) == 0 {
				out.Print("  (none)\n")
			} else {
				for _, c := range cachedMatches {
					out.Print("  %s/%s:%s (%d assets)\n", c.Namespace, c.Slug, c.Version, c.AssetCount)
				}
			}

			out.Println()
			out.Println("Installed in current project:")

			if len(installedMatches) == 0 {
				out.Print("  (none)\n")
			} else {
				for i := range installedMatches {
					out.Print("  %s:%s [%s] (%d assets)\n", installedMatches[i].Ref, installedMatches[i].Version, installedMatches[i].Harness, len(installedMatches[i].Assets))
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
		Use:   "uninstall <namespace/slug>[:<version>] --harness <type>",
		Short: "Remove installed bundle assets from the current project",
		Long: `Remove previously installed bundle assets from the current project directory.

Lists the files that will be removed and prompts for confirmation unless
--force is passed.`,
		Example: `  mush bundle uninstall acme/my-kit --harness claude
  mush bundle uninstall acme/my-kit:1.0.0 --harness claude --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			ref, err := bundle.ParseRef(strings.TrimSpace(args[0]))
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use: mush bundle uninstall <namespace/slug>[:<version>] --harness <type>",
					Code:    clierrors.ExitUsage,
				}
			}

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
			entry, err := bundle.FindInstalled(workDir, ref, normalized)
			if err != nil {
				if errors.Is(err, bundle.ErrNotInstalled) {
					return clierrors.BundleNotFound(ref.String())
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
					fmt.Sprintf("Uninstall %s (%s)? This will remove %d file(s)", ref.String(), normalized, len(entry.Assets)),
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

			removed, err := bundle.Uninstall(workDir, ref, normalized)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to uninstall bundle assets", err)
			}

			for _, relPath := range removed {
				out.Success("Removed: %s", relPath)
			}

			out.Println()
			out.Success("Uninstalled %s for harness %s", ref.String(), normalized)

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

// isForbiddenError returns true if the error chain contains an HTTP 403 status,
// indicating the resource requires authentication.
func isForbiddenError(err error) bool {
	return strings.Contains(err.Error(), "status 403")
}
