package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/tui/nav"
)

type rootRuntimeState struct {
	out *output.Writer
}

func configureRootRuntime(
	cmd *cobra.Command,
	out *output.Writer,
	jsonOutput bool,
	quiet bool,
	noInput bool,
	noColor bool,
	logLevel string,
	logFormat string,
	logFile string,
	logStderr string,
) (*rootRuntimeState, error) {
	out.JSON = pickBoolFlagOrEnv(jsonOutput, "MUSH_JSON", "MUSH_JSON")
	out.Quiet = pickBoolFlagOrEnv(quiet, "MUSH_QUIET", "MUSH_QUIET")
	out.NoInput = pickBoolFlagOrEnv(noInput, "MUSH_NO_INPUT", "MUSH_NO_INPUT") || pickBoolFlagOrEnv(false, "CI")

	if noColor {
		out.SetNoColor(true)
	}

	logCfg := observability.Config{
		Level:          pickFlagOrEnv(logLevel, "MUSH_LOG_LEVEL", "info"),
		Format:         pickFlagOrEnv(logFormat, "MUSH_LOG_FORMAT", "json"),
		LogFile:        pickFlagOrEnv(logFile, "MUSH_LOG_FILE", ""),
		StderrMode:     pickFlagOrEnv(logStderr, "MUSH_LOG_STDERR", "auto"),
		InteractiveTTY: out.Terminal().IsTTY && isInteractiveCommand(cmd.CommandPath()),
		SessionID:      uuid.NewString(),
		CommandPath:    cmd.CommandPath(),
		Version:        version,
		Commit:         commit,
	}

	logger, cleanup, err := observability.NewLogger(&logCfg)
	if err != nil {
		return nil, &clierrors.CLIError{
			Message: fmt.Sprintf("Invalid logging configuration: %v", err),
			Hint:    "Use --log-level (error|warn|info|debug), --log-format (json|text), --log-stderr (auto|on|off), and/or --log-file",
			Code:    clierrors.ExitUsage,
		}
	}

	slog.SetDefault(logger)

	ctx := out.WithContext(cmd.Context())
	ctx = observability.WithLogger(ctx, logger)
	cmd.SetContext(ctx)

	if cleanup != nil {
		cmd.PostRunE = wrapPostRunCleanup(cmd.PostRunE, cleanup)
	}

	telemetryCfg := &observability.TelemetryConfig{
		Enabled: observability.IsTelemetryEnabled(),
		Version: version,
		Commit:  commit,
	}

	telemetryShutdown, telemetryErr := observability.SetupTelemetry(ctx, telemetryCfg)
	if telemetryErr != nil {
		logger.Warn("telemetry initialization failed", "error", telemetryErr.Error())
	}

	if telemetryShutdown != nil {
		cmd.PostRunE = wrapNamedPostRunCleanup(cmd.PostRunE, "telemetry resources", func() error {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			return telemetryShutdown(shutdownCtx)
		})
	}

	return &rootRuntimeState{out: out}, nil
}

func wrapPostRunCleanup(postRun func(*cobra.Command, []string) error, cleanup func() error) func(*cobra.Command, []string) error {
	return wrapNamedPostRunCleanup(postRun, "logger resources", cleanup)
}

func wrapNamedPostRunCleanup(postRun func(*cobra.Command, []string) error, name string, cleanup func() error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if postRun != nil {
			if err := postRun(cmd, args); err != nil {
				_ = cleanup()
				return err
			}
		}

		if err := cleanup(); err != nil {
			return clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("cleanup %s", name), err)
		}

		return nil
	}
}

func pickBoolFlagOrEnv(flagValue bool, envKeys ...string) bool {
	if flagValue {
		return true
	}

	for _, envKey := range envKeys {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
		if v == "1" || v == "true" || v == "yes" {
			return true
		}
	}

	return false
}

func pickFlagOrEnv(flagValue, envKey, fallback string) string {
	trimmed := strings.TrimSpace(flagValue)
	if trimmed != "" {
		return trimmed
	}

	if envValue := strings.TrimSpace(os.Getenv(envKey)); envValue != "" {
		return envValue
	}

	return fallback
}

// experimentalOn returns true if experimental features are enabled via flag, env, or config.
func experimentalOn() bool {
	if pickBoolFlagOrEnv(false, "MUSH_EXPERIMENTAL", "MUSH_EXPERIMENTAL") {
		return true
	}

	return config.Load().Experimental()
}

// hasExperimentalFlag scans os.Args for the --experimental flag.
func hasExperimentalFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--experimental" || strings.HasPrefix(arg, "--experimental=") {
			return true
		}

		if arg == "--" {
			break
		}
	}

	return false
}

func isInteractiveCommand(path string) bool {
	return path == "mush worker start" || strings.HasPrefix(path, "mush worker start ") ||
		path == "mush bundle load" || strings.HasPrefix(path, "mush bundle load ") ||
		path == "mush bundle run" || strings.HasPrefix(path, "mush bundle run ")
}

// buildTUIDeps creates the Dependencies struct for the TUI from available auth/config.
func buildTUIDeps() *nav.Dependencies {
	cfg := config.Load()
	deps := &nav.Dependencies{
		Config:       cfg,
		Experimental: experimentalOn() || hasExperimentalFlag(),
	}

	source, apiKey := auth.GetCredentials(cfg.APIURL())
	if source == auth.SourceNone || apiKey == "" {
		apiKey = ""
	}

	httpClient, err := client.NewInstrumentedHTTPClient(cfg.CACertFile())
	if err == nil {
		deps.Client = client.NewWithHTTPClient(cfg.APIURL(), apiKey, httpClient)
	}

	if wd, err := os.Getwd(); err == nil {
		deps.WorkDir = wd
	}

	return deps
}

// shouldShowTUI returns true if the interactive TUI should be launched.
func shouldShowTUI(noTUI bool, out *output.Writer) bool {
	if out.JSON || out.Quiet || out.NoInput {
		return false
	}

	if !out.Terminal().IsTTY {
		return false
	}

	if pickBoolFlagOrEnv(noTUI, "MUSH_NO_TUI", "MUSH_NO_TUI") {
		return false
	}

	return config.Load().TUI()
}
