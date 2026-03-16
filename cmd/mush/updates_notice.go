package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/executil"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/update"
)

// skipUpdateCommands are commands that should not trigger background checks or show update notifications.
var skipUpdateCommands = map[string]bool{
	"update":     true,
	"version":    true,
	"completion": true,
	"doctor":     true,
	"__ua":       true,
}

// shouldBackgroundCheck returns true if a background update check should be launched.
func shouldBackgroundCheck(cmd *cobra.Command, ver string, out *output.Writer) bool {
	if ver == "dev" || out.Quiet || out.JSON || isUpdateDisabled() {
		return false
	}

	return !skipUpdateCommands[cmd.Name()]
}

func launchDetachedUpdateAgent() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}

	cmd, err := executil.AbsoluteCommandContext(context.Background(), execPath, "__ua", "--quiet", "--no-input", "--no-color")
	if err != nil {
		return
	}

	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return
	}

	go func() {
		_ = cmd.Wait()
	}()
}

// shouldShowUpdateNotice returns true if an update notice should be shown after command execution.
func shouldShowUpdateNotice(cmd *cobra.Command, ver string, out *output.Writer) bool {
	if ver == "dev" || out.Quiet || out.JSON || isUpdateDisabled() {
		return false
	}

	return !skipUpdateCommands[cmd.Name()]
}

// showUpdateNotice reads the cached state and prints an update notice if available.
func showUpdateNotice(out *output.Writer, currentVersion string) {
	state, err := update.LoadState()
	if err != nil {
		return
	}

	if !state.HasUpdate(currentVersion) {
		return
	}

	out.Print("\n")
	out.Info("A new version of mush is available: v%s → v%s", currentVersion, state.LatestVersion)

	source := update.InstallSource(state.InstallSource)
	if state.AutoApplyBlockedReason == "managed_install" {
		if hint := update.UpgradeHint(source); hint != "" {
			out.Muted("  Installed via %s. Run '%s'", state.InstallSource, hint)
			return
		}
	}

	if state.StagedVersion != "" && state.AutoApplyBlockedReason == "" {
		if state.LastApplyError != "" {
			out.Muted("  Run 'mush update' to update")
		} else {
			out.Muted("  Update staged in background and will apply on a future run")
		}

		return
	}

	out.Muted("  Run 'mush update' to update")
}
