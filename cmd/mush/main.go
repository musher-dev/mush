// Package main is the entry point for the Mush CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/buildinfo"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/update"
)

// Version information (set via ldflags during build).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	// Restore cursor visibility on panic to prevent hidden cursor if process crashes during spinner
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprint(os.Stderr, "\033[?25h") // Show cursor (ANSI escape sequence) - use stderr as it's unbuffered
			panic(r)
		}
	}()

	// Set runner version from build-time ldflags
	buildinfo.Version = version

	out := output.Default()

	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		return handleError(out, err)
	}

	return 0
}

// handleError formats and displays a CLI error, returning the appropriate exit code.
// For CLIError types, it displays the message and hint with styled output.
// For Cobra errors (unknown command, flags), it prints them with suggestions.
func handleError(out *output.Writer, err error) int {
	var cliErr *clierrors.CLIError
	if clierrors.As(err, &cliErr) {
		// CLIErrors are our custom errors - print with styled output
		out.Failure("%s", cliErr.Message)

		if cliErr.Hint != "" {
			out.Info("%s", cliErr.Hint)
		}

		return cliErr.Code
	}

	errStr := err.Error()

	// Handle Cobra's unknown command errors with suggestions
	// Format: "unknown command \"xyz\" for \"mush\"\n\nDid you mean this?\n\t..."
	if strings.HasPrefix(errStr, "unknown command") {
		out.Failure("%s", errStr)

		if !strings.Contains(errStr, "--help") {
			out.Info("Run 'mush --help' for usage")
		}

		return clierrors.ExitUsage
	}

	// Handle other Cobra errors (safety net — flag errors are normally
	// wrapped as CLIError by SetFlagErrorFunc, but standalone commands
	// without a parent may still reach here).
	if strings.HasPrefix(errStr, "unknown flag") ||
		strings.HasPrefix(errStr, "unknown shorthand flag") ||
		strings.Contains(errStr, "required flag") {
		out.Failure("%s", errStr)
		out.Info("Run 'mush --help' for usage")

		return clierrors.ExitUsage
	}

	// Other errors - print with styled output
	out.Failure("%s", errStr)

	return clierrors.ExitGeneral
}

func newRootCmd() *cobra.Command {
	var (
		jsonOutput bool
		quiet      bool
		noColor    bool
		noInput    bool
		verbose    bool
	)

	out := output.Default()

	rootCmd := &cobra.Command{
		Use:   "mush",
		Short: "Mush - Local worker runtime for Musher",
		Long: `Mush is a local worker runtime that connects your machine to the
Musher job stream. It claims jobs, runs handlers locally
(with access to your dev environment), and posts results back.

The golden path:
  Linear Issue → Musher Queue → Mush (local) → Claude Code → Result

Get started:
  mush init             Setup Mush for first use
  mush auth login       Authenticate with your API key
  mush habitat list     View available habitats
  mush link             Link to a habitat and start processing
  mush doctor           Diagnose common issues`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Configure output based on flags
			out.JSON = jsonOutput
			out.Quiet = quiet
			out.Verbose = verbose
			out.NoInput = noInput

			if noColor {
				out.SetNoColor(true)

				color.NoColor = true
			}

			// Store writer in context for subcommands
			cmd.SetContext(out.WithContext(cmd.Context()))

			// Launch background update check; tracked by updateWg so PostRunE
			// can wait for the state file write before reading it.
			if shouldBackgroundCheck(cmd, version, quiet, jsonOutput) {
				updateWg.Go(func() {
					backgroundUpdateCheck(version)
				})
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			// Wait for the background update goroutine to finish writing
			// the state file so we can read fresh results.
			updateWg.Wait()

			if shouldShowUpdateNotice(cmd, version, quiet, jsonOutput) {
				showUpdateNotice(out, version)
			}

			return nil
		},
	}

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Minimal output (for CI)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&noInput, "no-input", false, "Disable interactive prompts")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")

	// Enable typo suggestions for unknown commands
	rootCmd.SuggestionsMinimumDistance = 2

	// Wrap Cobra's raw flag errors in CLIError so they get styled output
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &clierrors.CLIError{
			Message: err.Error(),
			Hint:    fmt.Sprintf("Run '%s --help' for available flags", cmd.CommandPath()),
			Code:    clierrors.ExitUsage,
		}
	})

	// Primary commands
	rootCmd.AddCommand(newLinkCmd())
	rootCmd.AddCommand(newUnlinkCmd())
	rootCmd.AddCommand(newHabitatCmd())
	rootCmd.AddCommand(newBundleCmd())

	// Resource commands (noun-first)
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newHistoryCmd())

	// Utility commands
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newCompletionCmd())

	return rootCmd
}

// VersionInfo represents version information for JSON output.
type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// noArgs returns a Cobra positional-arg validator that rejects any arguments
// with a clear, user-friendly message (unlike cobra.NoArgs which says "unknown command").
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return &clierrors.CLIError{
			Message: fmt.Sprintf("'%s' accepts no arguments", cmd.CommandPath()),
			Hint:    fmt.Sprintf("Run '%s --help' for usage", cmd.CommandPath()),
			Code:    clierrors.ExitUsage,
		}
	}

	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			if out.JSON {
				return out.PrintJSON(VersionInfo{
					Version: version,
					Commit:  commit,
					Date:    date,
				})
			}

			out.Print("mush %s\n", version)
			out.Print("  commit: %s\n", commit)
			out.Print("  built:  %s\n", date)

			return nil
		},
	}
}

// updateWg tracks the background update goroutine so PersistentPostRunE can
// wait for it to finish writing the state file before reading it.
var updateWg sync.WaitGroup

// skipUpdateCommands are commands that should not trigger background checks or show update notifications.
var skipUpdateCommands = map[string]bool{
	"update":     true,
	"version":    true,
	"completion": true,
	"doctor":     true,
}

// shouldBackgroundCheck returns true if a background update check should be launched.
func shouldBackgroundCheck(cmd *cobra.Command, ver string, quiet, jsonOut bool) bool {
	if ver == "dev" || quiet || jsonOut || isUpdateDisabled() {
		return false
	}

	return !skipUpdateCommands[cmd.Name()]
}

// backgroundUpdateCheck performs the update check in a goroutine and saves state.
func backgroundUpdateCheck(currentVersion string) {
	state, err := update.LoadState()
	if err != nil {
		return
	}

	if !state.ShouldCheck() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	updater, err := update.NewUpdater()
	if err != nil {
		return
	}

	info, err := updater.CheckLatest(ctx, currentVersion)
	if err != nil {
		return
	}

	_ = update.SaveState(&update.State{
		LastCheckedAt:  time.Now(),
		LatestVersion:  info.LatestVersion,
		CurrentVersion: currentVersion,
		ReleaseURL:     info.ReleaseURL,
	})
}

// shouldShowUpdateNotice returns true if an update notice should be shown after command execution.
func shouldShowUpdateNotice(cmd *cobra.Command, ver string, quiet, jsonOut bool) bool {
	if ver == "dev" || quiet || jsonOut || isUpdateDisabled() {
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

	if state.HasUpdate(currentVersion) {
		out.Print("\n")
		out.Info("A new version of mush is available: v%s → v%s", currentVersion, state.LatestVersion)
		out.Muted("  Run 'mush update' to update")
	}
}
