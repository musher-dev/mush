//go:build unix

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/safeio"
	"github.com/musher-dev/mush/internal/tui/nav"
)

func handleBundleLoadNavResult(cmd *cobra.Command, out *output.Writer, result *nav.Result) error {
	if !out.Terminal().IsTTY {
		return &clierrors.CLIError{
			Message: "Bundle launch requires a terminal (TTY)",
			Hint:    "Run this command directly in a terminal, not in a pipe or script",
			Code:    clierrors.ExitUsage,
		}
	}

	if result.CachePath == "" {
		return &clierrors.CLIError{
			Message: "Missing bundle cache path from navigation result",
			Hint:    "Re-run the bundle flow and launch again",
			Code:    clierrors.ExitGeneral,
		}
	}

	normalized, err := validateNavHarness(result.Harness)
	if err != nil {
		return err
	}

	resolved, err := loadCachedBundleResolve(result.CachePath)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read cached bundle manifest", err).
			WithHint("Re-run the bundle flow to refresh cache")
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
		return clierrors.New(clierrors.ExitGeneral,
			fmt.Sprintf("No provider spec for harness: %s", normalized))
	}

	session, err := bundle.PrepareLoadSession(
		cmd.Context(), projectDir, result.CachePath, &resolved.Manifest, spec, mapper,
	)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prepare bundle load session", err).
			WithHint("Re-run the bundle flow or check the log file for details")
	}

	defer session.Cleanup()

	for _, w := range session.Warnings {
		out.Warning("%s", w)
	}

	for _, relPath := range session.Prepared {
		out.Success("Prepared: %s", relPath)
	}

	runnerConfig := loadRunnerConfigIfAvailable(cmd, out)

	cfg := &harness.Config{
		SupportedHarnesses: []string{normalized},
		BundleLoadMode:     true,
		BundleName:         resolved.Slug,
		BundleVer:          resolved.Version,
		BundleDir:          session.BundleDir,
		BundleWorkDir:      session.WorkingDir,
		BundleEnv:          session.Env,
		RunnerConfig:       runnerConfig,
		BundleSummary:      harness.SummarizeBundleManifest(&resolved.Manifest),
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	if err := harness.Run(ctx, cfg); err != nil {
		return clierrors.Wrap(clierrors.ExitExecution, "Bundle launch failed", err)
	}

	return nil
}

func handleBareRunNavResult(cmd *cobra.Command, out *output.Writer, result *nav.Result) error {
	if !out.Terminal().IsTTY {
		return &clierrors.CLIError{
			Message: "Launching an interaction requires a terminal (TTY)",
			Hint:    "Run this command directly in a terminal, not in a pipe or script",
			Code:    clierrors.ExitUsage,
		}
	}

	normalized, err := validateNavHarness(result.Harness)
	if err != nil {
		return err
	}

	cfg := &harness.Config{
		SupportedHarnesses: []string{normalized},
		BundleLoadMode:     true,
		RunnerConfig:       loadRunnerConfigIfAvailable(cmd, out),
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	if err := harness.Run(ctx, cfg); err != nil {
		return clierrors.Wrap(clierrors.ExitExecution, "Interaction launch failed", err)
	}

	return nil
}

func validateNavHarness(raw string) (string, error) {
	if raw == "" {
		return "", &clierrors.CLIError{
			Message: "Harness type is required",
			Hint:    fmt.Sprintf("Select a harness before launching. Available: %s", joinNames(harness.RegisteredNames())),
			Code:    clierrors.ExitUsage,
		}
	}

	normalized, err := normalizeHarnessType(raw)
	if err != nil {
		return "", err
	}

	info, ok := harness.Lookup(normalized)
	if !ok || !info.Available() {
		return "", clierrors.HarnessNotAvailable(normalized)
	}

	return normalized, nil
}

func loadCachedBundleResolve(cachePath string) (*client.BundleResolveResponse, error) {
	manifestPath := filepath.Join(cachePath, "manifest.json")

	data, err := safeio.ReadFile(manifestPath)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("read %s", manifestPath), err)
	}

	var resolved client.BundleResolveResponse
	if err := json.Unmarshal(data, &resolved); err != nil {
		return nil, clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("parse %s", manifestPath), err)
	}

	return &resolved, nil
}

func handleBundleInstallNavResult(_ *cobra.Command, out *output.Writer, result *nav.Result) error {
	if result.CachePath == "" {
		return &clierrors.CLIError{
			Message: "Missing bundle cache path from navigation result",
			Hint:    "Re-run the bundle flow and install again",
			Code:    clierrors.ExitGeneral,
		}
	}

	normalized, err := validateNavHarness(result.Harness)
	if err != nil {
		return err
	}

	resolved, err := loadCachedBundleResolve(result.CachePath)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read cached bundle manifest", err).
			WithHint("Re-run the bundle flow to refresh cache")
	}

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

	installed, installErr := bundle.InstallFromCache(workDir, result.CachePath, &resolved.Manifest, mapper, result.Force)
	if installErr != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Bundle install failed", installErr)
	}

	for _, relPath := range installed {
		out.Success("Installed: %s", relPath)
	}

	ref := result.BundleNamespace + "/" + result.BundleSlug

	trackErr := bundle.TrackInstall(workDir, &bundle.InstalledBundle{
		Namespace: result.BundleNamespace,
		Slug:      result.BundleSlug,
		Ref:       ref,
		Version:   result.BundleVer,
		Harness:   normalized,
		Assets:    installed,
	})
	if trackErr != nil {
		out.Warning("Failed to track install: %v", trackErr)
	}

	out.Success("Bundle %s v%s installed for %s", ref, result.BundleVer, normalized)

	return nil
}

func loadRunnerConfigIfAvailable(cmd *cobra.Command, out *output.Writer) *client.RunnerConfigResponse {
	_, c, _, err := tryAPIClient()
	if err != nil || c == nil || !c.IsAuthenticated() {
		return nil
	}

	runnerConfig, cfgErr := c.GetRunnerConfig(cmd.Context())
	if cfgErr != nil {
		out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", cfgErr)
		return nil
	}

	return runnerConfig
}
