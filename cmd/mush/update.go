package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/update"
)

func newUpdateCmd() *cobra.Command {
	var (
		targetVersion string
		force         bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update mush to the latest version",
		Long: `Update mush to the latest version from GitHub Releases.

Downloads the new binary, verifies its checksum, and replaces the current
executable. If the binary is not writable, sudo is requested automatically.

Set MUSH_UPDATE_DISABLED=1 to disable update checks.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			return runUpdate(cmd, out, targetVersion, force)
		},
	}

	cmd.Flags().StringVar(&targetVersion, "version", "", "Install a specific version (e.g. 1.2.3)")
	cmd.Flags().BoolVar(&force, "force", false, "Force update even if already up to date")

	return cmd
}

func runUpdate(cmd *cobra.Command, out *output.Writer, targetVersion string, force bool) error {
	ctx := cmd.Context()

	// Check if updates are disabled
	if isUpdateDisabled() {
		out.Warning("Updates are disabled (MUSH_UPDATE_DISABLED is set)")
		return nil
	}

	currentVersion := buildinfo.Version

	// Dev builds can't be updated
	if currentVersion == "dev" && targetVersion == "" {
		out.Warning("Development build — cannot determine current version")
		out.Info("Install a release build: https://github.com/musher-dev/mush/releases")

		return nil
	}

	updater, err := update.NewUpdater()
	if err != nil {
		return fmt.Errorf("failed to initialize updater: %w", err)
	}

	// Specific version mode
	if targetVersion != "" {
		targetVersion = strings.TrimPrefix(targetVersion, "v")
		return updateToVersion(ctx, out, updater, targetVersion)
	}

	// Check for latest (skip spinner in JSON mode to avoid corrupting stdout)
	var spin *output.Spinner
	if !out.JSON {
		spin = out.Spinner("Checking for updates")
		spin.Start()
	}

	info, err := updater.CheckLatest(ctx, currentVersion)
	if err != nil {
		if spin != nil {
			spin.StopWithFailure(fmt.Sprintf("Failed to check for updates: %v", err))
		}

		if strings.Contains(err.Error(), "403") {
			out.Info("Set GITHUB_TOKEN to avoid rate limits")
		}

		return fmt.Errorf("update check failed: %w", err)
	}

	// JSON output mode — print check result and exit without applying
	if out.JSON {
		if printErr := out.PrintJSON(info); printErr != nil {
			return fmt.Errorf("print update info as json: %w", printErr)
		}

		return nil
	}

	if !info.UpdateAvailable && !force {
		spin.StopWithSuccess(fmt.Sprintf("Already up to date (v%s)", currentVersion))
		saveCheckState(currentVersion, info.LatestVersion, info.ReleaseURL)

		return nil
	}

	// Guard against nil Release (no matching platform assets found)
	if info.Release == nil {
		spin.StopWithFailure("No release found for this platform")
		return fmt.Errorf("no release found for this platform")
	}

	if info.UpdateAvailable {
		spin.StopWithSuccess(fmt.Sprintf("Update available: v%s → v%s", currentVersion, info.LatestVersion))
	} else {
		spin.StopWithSuccess(fmt.Sprintf("Reinstalling v%s", info.LatestVersion))
	}

	// Check write permissions and re-exec with sudo if needed
	execPath, err := selfupdate.ExecutablePath()
	if err == nil && update.NeedsElevation(execPath) {
		if sudoErr := update.ReExecWithSudo(); sudoErr != nil {
			return fmt.Errorf("re-exec updater with sudo: %w", sudoErr)
		}

		return nil
	}

	// Apply update
	spin = out.Spinner(fmt.Sprintf("Downloading v%s", info.LatestVersion))
	spin.Start()

	if err := updater.Apply(ctx, info.Release); err != nil {
		spin.StopWithFailure(fmt.Sprintf("Update failed: %v", err))
		return fmt.Errorf("update failed: %w", err)
	}

	spin.StopWithSuccess(fmt.Sprintf("Updated to v%s", info.LatestVersion))

	if info.ReleaseURL != "" {
		out.Muted("Release notes: %s", info.ReleaseURL)
	}

	saveCheckState(currentVersion, info.LatestVersion, info.ReleaseURL)

	return nil
}

func updateToVersion(ctx context.Context, out *output.Writer, updater *update.Updater, version string) error {
	// Check write permissions and re-exec with sudo if needed
	execPath, err := selfupdate.ExecutablePath()
	if err == nil && update.NeedsElevation(execPath) {
		if sudoErr := update.ReExecWithSudo(); sudoErr != nil {
			return fmt.Errorf("re-exec updater with sudo: %w", sudoErr)
		}

		return nil
	}

	// Skip spinner in JSON mode to avoid corrupting stdout
	var spin *output.Spinner
	if !out.JSON {
		spin = out.Spinner(fmt.Sprintf("Installing v%s", version))
		spin.Start()
	}

	release, err := updater.ApplyVersion(ctx, version)
	if err != nil {
		if spin != nil {
			spin.StopWithFailure(fmt.Sprintf("Failed to install v%s: %v", version, err))
		}

		if strings.Contains(err.Error(), "not found") {
			out.Info("Check available versions at https://github.com/musher-dev/mush/releases")
		}

		return fmt.Errorf("install failed: %w", err)
	}

	if spin != nil {
		spin.StopWithSuccess(fmt.Sprintf("Installed v%s", release.Version()))
	}

	return nil
}

func saveCheckState(current, latest, releaseURL string) {
	state := &update.State{
		LastCheckedAt:  time.Now(),
		LatestVersion:  latest,
		CurrentVersion: current,
		ReleaseURL:     releaseURL,
	}
	_ = update.SaveState(state)
}

func isUpdateDisabled() bool {
	return update.IsDisabled()
}
