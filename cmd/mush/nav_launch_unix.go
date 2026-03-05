//go:build unix

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/output"
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

	normalized, err := validateNavHarness(result.Harness)
	if err != nil {
		return err
	}

	if result.CachePath == "" {
		return &clierrors.CLIError{
			Message: "Missing bundle cache path from navigation result",
			Hint:    "Re-run the bundle flow and launch again",
			Code:    clierrors.ExitGeneral,
		}
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

	tmpDir, cleanup, err := mapper.PrepareLoad(cmd.Context(), result.CachePath, &resolved.Manifest)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prepare load directory", err)
	}

	defer cleanup()

	projectDir, err := os.Getwd()
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
	}

	injected, assetWarnings, assetCleanup, err := bundle.InjectAssetsForLoad(
		projectDir, result.CachePath, &resolved.Manifest, mapper,
	)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to inject assets for load", err)
	}

	defer assetCleanup()

	for _, w := range assetWarnings {
		out.Warning("%s", w)
	}

	for _, relPath := range injected {
		out.Success("Injected: %s", relPath)
	}

	spec, _ := harness.GetProvider(normalized)
	if spec != nil && (spec.CLI == nil || spec.CLI.MCPConfig == "") {
		toolInjected, toolCleanup, toolErr := bundle.InjectToolConfigsForLoad(
			projectDir, result.CachePath, &resolved.Manifest, mapper,
		)
		if toolErr != nil {
			return clierrors.Wrap(clierrors.ExitGeneral, "Failed to inject tool configs for load", toolErr)
		}

		defer toolCleanup()

		for _, relPath := range toolInjected {
			out.Success("Injected: %s", relPath)
		}
	}

	runnerConfig := loadRunnerConfigIfAvailable(cmd, out)

	cfg := &harness.Config{
		SupportedHarnesses: []string{normalized},
		BundleLoadMode:     true,
		BundleName:         resolved.Slug,
		BundleVer:          resolved.Version,
		BundleDir:          tmpDir,
		RunnerConfig:       runnerConfig,
		BundleSummary:      harness.SummarizeBundleManifest(&resolved.Manifest),
	}

	if err := harness.Run(cmd.Context(), cfg); err != nil {
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

	if err := harness.Run(cmd.Context(), cfg); err != nil {
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

	data, err := os.ReadFile(manifestPath) //nolint:gosec // path is from controlled bundle cache.
	if err != nil {
		return nil, clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("read %s", manifestPath), err)
	}

	var resolved client.BundleResolveResponse
	if err := json.Unmarshal(data, &resolved); err != nil {
		return nil, clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("parse %s", manifestPath), err)
	}

	return &resolved, nil
}

func loadRunnerConfigIfAvailable(cmd *cobra.Command, out *output.Writer) *client.RunnerConfigResponse {
	_, c, _, err := tryAPIClient()
	if err != nil || c == nil {
		return nil
	}

	runnerConfig, cfgErr := c.GetRunnerConfig(cmd.Context())
	if cfgErr != nil {
		out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", cfgErr)
		return nil
	}

	return runnerConfig
}
