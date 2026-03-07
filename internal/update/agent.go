package update

import (
	"context"
	"fmt"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// AgentConfig controls background update behavior.
type AgentConfig struct {
	CurrentVersion string
	CheckInterval  time.Duration
	AutoApply      bool
}

// RunAgent performs a single background update tick.
func RunAgent(cfg AgentConfig) error {
	if IsDisabled() || cfg.CurrentVersion == "" || cfg.CurrentVersion == "dev" {
		return nil
	}

	return WithAgentLock(func() error {
		state, err := LoadState()
		if err != nil {
			return err
		}

		execPath, execErr := selfupdate.ExecutablePath()
		source := InstallSourceUnknown
		if execErr == nil {
			source = DetectInstallSource(execPath)
		}

		state.InstallSource = string(source)
		state.AutoApplyBlockedReason = ""

		allowedBySource := AutoApplyAllowed(source)
		if !allowedBySource {
			state.AutoApplyBlockedReason = "managed_install"
		}

		if cfg.AutoApply && allowedBySource && state.HasStagedUpdate(cfg.CurrentVersion) {
			if err := applyStaged(state, execPath); err == nil {
				return nil
			}
		}

		if !state.ShouldCheck(cfg.CheckInterval) {
			return SaveState(state)
		}

		checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		updater, err := NewUpdater()
		if err != nil {
			return SaveState(state)
		}

		info, err := updater.CheckLatest(checkCtx, cfg.CurrentVersion)
		if err != nil {
			return SaveState(state)
		}

		now := time.Now()
		state.LastCheckedAt = now
		state.LatestVersion = info.LatestVersion
		state.CurrentVersion = cfg.CurrentVersion
		state.ReleaseURL = info.ReleaseURL

		if info.UpdateAvailable {
			state.StagedVersion = info.LatestVersion
			state.StagedAt = now
			if !cfg.AutoApply {
				state.AutoApplyBlockedReason = "auto_apply_disabled"
			} else if !allowedBySource {
				state.AutoApplyBlockedReason = "managed_install"
			}
		} else {
			state.ClearStaged()
			state.LastApplyError = ""
		}

		return SaveState(state)
	})
}

func applyStaged(state *State, execPath string) error {
	if execPath == "" {
		return fmt.Errorf("executable path unavailable")
	}

	if NeedsElevation(execPath) {
		state.LastApplyAttemptAt = time.Now()
		state.LastApplyError = "background apply requires elevated permissions"
		state.AutoApplyBlockedReason = "elevation_required"
		return SaveState(state)
	}

	applyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	updater, err := NewUpdater()
	if err != nil {
		return err
	}

	_, err = updater.ApplyVersion(applyCtx, state.StagedVersion)
	state.LastApplyAttemptAt = time.Now()
	if err != nil {
		state.LastApplyError = err.Error()
		return SaveState(state)
	}

	state.LastApplyError = ""
	state.CurrentVersion = state.StagedVersion
	state.LatestVersion = state.StagedVersion
	state.AutoApplyBlockedReason = ""
	state.ClearStaged()

	return SaveState(state)
}
