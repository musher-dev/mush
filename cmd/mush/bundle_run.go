//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/executil"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/observability"
	"github.com/musher-dev/mush/internal/output"
)

func newBundleRunCmd() *cobra.Command {
	var (
		harnessType string
		dirPath     string
		useSample   bool
	)

	cmd := &cobra.Command{
		Use:   "run [<namespace/slug>[:<version>]] --harness <type>",
		Short: "Run a bundle directly with a harness",
		Long: `Pull a bundle and launch the harness binary directly as a subprocess with
inherited stdio. No TUI, no PTY wrapping — you get the raw harness
experience as if you ran it yourself.

Use --dir to load a bundle from a local directory or --sample to use the
built-in sample bundle for testing.`,
		Example: `  mush bundle run acme/my-kit --harness claude
  mush bundle run acme/my-kit:0.1.0 --harness claude
  mush bundle run --dir ./my-bundle --harness claude
  mush bundle run --sample --harness claude`,
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
				slog.String("event.type", "bundle.run.start"),
			)

			if harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required for bundle run",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			return executeBundleRun(cmd, out, logger, harnessType, bundleSourceOptions{
				dirPath:   dirPath,
				useSample: useSample,
				refArg:    firstArg(args),
			})
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to use (required)")
	cmd.Flags().StringVar(&dirPath, "dir", "", "Load bundle from a local directory")
	cmd.Flags().BoolVar(&useSample, "sample", false, "Load the built-in sample bundle")
	cmd.MarkFlagsMutuallyExclusive("dir", "sample")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

// executeBundleRun resolves a bundle, prepares assets, and launches the harness
// as a direct subprocess with inherited stdio.
func executeBundleRun(
	cmd *cobra.Command,
	out *output.Writer,
	logger *slog.Logger,
	harnessType string,
	sourceOpts bundleSourceOptions,
) error {
	normalized, err := normalizeHarnessType(harnessType)
	if err != nil {
		return err
	}

	info, ok := harness.Lookup(normalized)
	if !ok || !info.Available() {
		return clierrors.HarnessNotAvailable(normalized)
	}

	spec, ok := harness.GetProvider(normalized)
	if !ok {
		return clierrors.New(clierrors.ExitGeneral, fmt.Sprintf("No provider spec for harness: %s", normalized))
	}

	mapper := mapperForHarness(normalized)
	if mapper == nil {
		return &clierrors.CLIError{
			Message: fmt.Sprintf("No asset mapper for harness type: %s", normalized),
			Hint:    "This harness type does not support bundle assets",
			Code:    clierrors.ExitUsage,
		}
	}

	source, err := resolveBundleSource(cmd.Context(), out, logger, sourceOpts)
	if err != nil {
		return err
	}

	defer source.Cleanup()

	projectDir, err := os.Getwd()
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
	}

	session, err := bundle.PrepareLoadSession(
		cmd.Context(), projectDir, source.CachePath, &source.Resolved.Manifest, spec, mapper,
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

	mcpConfigPath, mcpCleanup := provisionMCPConfig(cmd.Context(), out, info, spec)
	if mcpCleanup != nil {
		defer func() { _ = mcpCleanup() }()
	}

	return launchHarnessSubprocess(logger, spec, session, mcpConfigPath)
}

// provisionMCPConfig creates an ephemeral MCP config file if the harness
// supports MCP and the user is authenticated.
func provisionMCPConfig(
	ctx context.Context,
	out *output.Writer,
	info harness.Info,
	_ *harnesstype.ProviderSpec,
) (path string, cleanup func() error) {
	if info.MCPSpec == nil {
		return "", nil
	}

	_, apiClient, _, apiErr := tryAPIClient()
	if apiErr != nil || apiClient == nil || !apiClient.IsAuthenticated() {
		return "", nil
	}

	runnerConfig, rcErr := apiClient.GetRunnerConfig(ctx)
	if rcErr != nil {
		out.Warning("Runner config unavailable, continuing without MCP provisioning: %v", rcErr)
		return "", nil
	}

	if runnerConfig == nil {
		return "", nil
	}

	path, _, cleanup, mcpErr := harnesstype.CreateMCPConfigFile(slog.Default(), info.MCPSpec, runnerConfig, time.Now())
	if mcpErr != nil {
		out.Warning("MCP config disabled: %v", mcpErr)
		return "", nil
	}

	return path, cleanup
}

// launchHarnessSubprocess starts the harness binary as a child process with
// inherited stdio and waits for it to exit, forwarding signals and returning
// the child's exit code.
func launchHarnessSubprocess(
	logger *slog.Logger,
	spec *harnesstype.ProviderSpec,
	session *bundle.LoadSession,
	mcpConfigPath string,
) error {
	harnessArgs := buildRunArgs(spec, session.BundleDir, mcpConfigPath)

	binaryPath, err := executil.LookPath(spec.Binary)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("Harness binary %q not found", spec.Binary), err).
			WithHint(installHintFor(spec))
	}

	logger.Info("launching harness subprocess",
		slog.String("harness.binary", binaryPath),
		slog.Any("harness.args", harnessArgs),
		slog.String("harness.workdir", session.WorkingDir),
	)

	// Use context.Background so Go does not auto-kill the child on signal.
	// The child receives signals directly from the terminal (shared process group).
	child, err := executil.CommandContext(context.Background(), spec.Binary, harnessArgs...)
	if err != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, fmt.Sprintf("Harness binary %q not found", spec.Binary), err).
			WithHint(installHintFor(spec))
	}

	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Dir = session.WorkingDir
	child.Env = append(os.Environ(), session.Env...)

	if err := child.Start(); err != nil {
		return clierrors.Wrap(clierrors.ExitExecution, "Failed to start harness", err)
	}

	// Intercept signals so mush stays alive for cleanup. The child receives
	// them naturally from the terminal (shared process group).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Drain the signal channel and forward signals to the child so that
	// default termination behavior is preserved while allowing mush to
	// perform cleanup.
	go func() {
		for sig := range sigCh {
			// Best-effort forwarding; ignore errors if the child is already gone.
			_ = child.Process.Signal(sig)
		}
	}()

	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()

	waitErr := child.Wait()

	logger.Info("harness subprocess exited", slog.String("harness.binary", spec.Binary))

	if waitErr == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			code = 128
		}

		return &clierrors.CLIError{
			Message: fmt.Sprintf("Harness exited with code %d", code),
			Code:    code,
		}
	}

	return clierrors.Wrap(clierrors.ExitExecution, "Harness subprocess failed", waitErr)
}

// buildRunArgs constructs the CLI arguments for the harness binary based on
// the provider spec, bundle directory, and optional MCP config path.
//
// Provider-specific args (e.g. sandbox flags) are injected before bundle/MCP
// flags so that harness binaries receive them in the expected order.
func buildRunArgs(spec *harnesstype.ProviderSpec, bundleDir, mcpConfigPath string) []string {
	var args []string

	// Provider-specific extra args required for correct operation.
	switch spec.Name {
	case "codex", "gemini":
		// These harnesses require explicit sandbox mode when using --add-dir;
		// without this, they default to read-only and ignore the bundle dir.
		if bundleDir != "" {
			args = append(args, "--sandbox", "workspace-write")
		}
	}

	if bundleDir != "" && spec.BundleDir != nil && spec.BundleDir.Flag != "" {
		args = append(args, spec.BundleDir.Flag, bundleDir)
	}

	if mcpConfigPath != "" && spec.CLI != nil && spec.CLI.MCPConfig != "" {
		args = append(args, spec.CLI.MCPConfig, mcpConfigPath)
	}

	return args
}

// installHintFor returns a user-facing install hint from the provider spec.
func installHintFor(spec *harnesstype.ProviderSpec) string {
	if spec.Status != nil && spec.Status.InstallHint != "" {
		return fmt.Sprintf("Install with: %s", spec.Status.InstallHint)
	}

	return fmt.Sprintf("Ensure %q is installed and on your PATH", spec.Binary)
}
