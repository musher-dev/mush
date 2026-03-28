//go:build unix

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
)

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

			source, err := resolveBundleSource(cmd.Context(), out, logger, bundleSourceOptions{
				dirPath: dirPath,
				refArg:  firstArg(args),
			})
			if err != nil {
				return err
			}
			defer source.Cleanup()

			mapper := mapperForHarness(normalized)
			if mapper == nil {
				return &clierrors.CLIError{
					Message: fmt.Sprintf("No asset mapper for harness type: %s", normalized),
					Hint:    "This harness type does not support bundle assets",
					Code:    clierrors.ExitUsage,
				}
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installedPaths, installErr := bundle.InstallFromCache(workDir, &source.Resolved.Manifest, mapper, force)
			if installErr != nil {
				var conflict *bundle.InstallConflictError
				if errors.As(installErr, &conflict) {
					logger.Warn("bundle install conflict", slog.String("error", installErr.Error()))
					return clierrors.InstallConflict(conflict.Path)
				}

				logger.Error("bundle install failed", slog.String("error", installErr.Error()))

				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to install bundle assets", installErr)
			}

			for _, relPath := range installedPaths {
				out.Success("Installed: %s", relPath)
			}

			trackErr := bundle.TrackInstall(workDir, &bundle.InstalledBundle{
				Namespace: source.Ref.Namespace,
				Slug:      source.Ref.Slug,
				Ref:       source.Ref.Namespace + "/" + source.Ref.Slug,
				Version:   source.Resolved.Version,
				Harness:   normalized,
				Assets:    installedPaths,
				Timestamp: time.Now(),
			})
			if trackErr != nil {
				out.Warning("Failed to track installation: %v", trackErr)
			}

			out.Println()
			out.Success("Installed %d assets from %s v%s", len(source.Resolved.Manifest.Layers), source.Ref.Slug, source.Resolved.Version)
			logger.Info("bundle install completed", slog.String("bundle.version", source.Resolved.Version), slog.Int("bundle.asset_count", len(installedPaths)))

			return nil
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to install for (required)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing files")
	cmd.Flags().StringVar(&dirPath, "dir", "", "Install bundle from a local directory")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}
