package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/tui/nav"
	"github.com/musher-dev/mush/internal/validate"
)

func newRootCmd() *cobra.Command {
	var (
		jsonOutput bool
		quiet      bool
		noColor    bool
		noInput    bool
		noTUI      bool
		logLevel   string
		logFormat  string
		logFile    string
		logStderr  string
		apiURL     string
		apiKey     string
	)

	out := rootOutputFactory()

	rootCmd := &cobra.Command{
		Use:   "mush",
		Short: "Portable agent bundles for local coding agents",
		Long: `Load, install, and manage agent bundles from the Musher Hub.
Browse bundles, run them ephemerally, or install assets into
your project's harness directory.

Get started:  mush bundle load`,
		Example:       `  mush bundle load acme/my-kit`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if shouldShowTUI(noTUI, out) {
				deps := buildTUIDeps()

				result, err := nav.Run(cmd.Context(), deps)
				if err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Interactive TUI failed", err)
				}

				if result.Action == nav.ActionBundleLoad {
					return handleBundleLoadNavResult(cmd, out, result)
				}

				if result.Action == nav.ActionBareRun {
					return handleBareRunNavResult(cmd, out, result)
				}

				if result.Action == nav.ActionWorkerStart {
					return handleWorkerNavResult(cmd, out, result)
				}

				if result.Action == nav.ActionHarnessInstall {
					return handleHarnessInstall(cmd.Context(), out, result)
				}

				if result.Action == nav.ActionBundleInstall {
					return handleBundleInstallNavResult(cmd, out, result)
				}

				return nil
			}

			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(apiURL) != "" {
				validatedURL, err := validateAPIURL(apiURL)
				if err != nil {
					return &clierrors.CLIError{
						Message: fmt.Sprintf("Invalid API URL: %v", err),
						Hint:    "Use --api-url with a valid absolute URL, e.g. https://api.musher.dev",
						Code:    clierrors.ExitUsage,
					}
				}

				if setErr := os.Setenv("MUSHER_API_URL", validatedURL); setErr != nil {
					return &clierrors.CLIError{
						Message: fmt.Sprintf("Failed to apply API URL override: %v", setErr),
						Hint:    "Check your shell environment and try again",
						Code:    clierrors.ExitUsage,
					}
				}
			}

			if strings.TrimSpace(apiKey) != "" {
				if setErr := os.Setenv("MUSHER_API_KEY", apiKey); setErr != nil {
					return &clierrors.CLIError{
						Message: fmt.Sprintf("Failed to apply API key override: %v", setErr),
						Hint:    "Check your shell environment and try again",
						Code:    clierrors.ExitUsage,
					}
				}
			}

			runtimeState, err := configureRootRuntime(
				cmd, out, jsonOutput, quiet, noInput, noColor, logLevel, logFormat, logFile, logStderr,
			)
			if err != nil {
				return err
			}

			if shouldBackgroundCheck(cmd, version, runtimeState.out) {
				launchDetachedUpdateAgent()
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			if shouldShowUpdateNotice(cmd, version, out) {
				showUpdateNotice(out, version)
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Minimal output (for CI)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&noInput, "no-input", false, "Disable interactive prompts")
	rootCmd.PersistentFlags().BoolVar(&noTUI, "no-tui", false, "Disable interactive TUI navigation")
	rootCmd.PersistentFlags().Bool("experimental", false, "Enable experimental features")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: error, warn, info, debug")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "", "Log format: json, text")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Optional structured log file path")
	rootCmd.PersistentFlags().StringVar(&logStderr, "log-stderr", "", "Structured logging to stderr: auto, on, off")
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Override Musher API URL for this command")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key override (prefer MUSHER_API_KEY env var)")

	_ = rootCmd.PersistentFlags().MarkHidden("log-level")
	_ = rootCmd.PersistentFlags().MarkHidden("log-format")
	_ = rootCmd.PersistentFlags().MarkHidden("log-file")
	_ = rootCmd.PersistentFlags().MarkHidden("log-stderr")
	_ = rootCmd.PersistentFlags().MarkHidden("experimental")

	rootCmd.SuggestionsMinimumDistance = 2
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &clierrors.CLIError{
			Message: err.Error(),
			Hint:    fmt.Sprintf("Run '%s --help' for available flags", cmd.CommandPath()),
			Code:    clierrors.ExitUsage,
		}
	})

	registerRootCommands(rootCmd)

	return rootCmd
}

func registerRootCommands(rootCmd *cobra.Command) {
	experimentalEnabled := experimentalOn() || hasExperimentalFlag()

	groups := []*cobra.Group{
		{ID: "bundles", Title: "Bundle Commands:"},
		{ID: "account", Title: "Account & Configuration:"},
		{ID: "setup", Title: "Setup & Diagnostics:"},
		{ID: "advanced", Title: "Advanced:"},
	}

	if experimentalEnabled {
		groups = append(groups, &cobra.Group{ID: "experimental", Title: "Experimental:"})
	}

	rootCmd.AddGroup(groups...)

	bundleCmd := newBundleCmd()
	bundleCmd.GroupID = "bundles"
	rootCmd.AddCommand(bundleCmd)

	workerCmd := newWorkerCmd()
	workerCmd.GroupID = "advanced"
	rootCmd.AddCommand(workerCmd)

	habitatCmd := newHabitatCmd()
	habitatCmd.GroupID = "advanced"
	rootCmd.AddCommand(habitatCmd)

	authCmd := newAuthCmd()
	authCmd.GroupID = "account"
	rootCmd.AddCommand(authCmd)

	configCmd := newConfigCmd()
	configCmd.GroupID = "account"
	rootCmd.AddCommand(configCmd)

	historyCmd := newHistoryCmd()
	historyCmd.GroupID = "account"
	rootCmd.AddCommand(historyCmd)

	initCmd := newInitCmd()
	initCmd.GroupID = "setup"
	rootCmd.AddCommand(initCmd)

	doctorCmd := newDoctorCmd()
	doctorCmd.GroupID = "setup"
	rootCmd.AddCommand(doctorCmd)

	updateCmd := newUpdateCmd()
	updateCmd.GroupID = "setup"
	rootCmd.AddCommand(updateCmd)

	rootCmd.AddCommand(newUpdateAgentCmd())

	experimentalCmd := newExperimentalCmd()
	experimentalCmd.Hidden = !experimentalEnabled

	if experimentalEnabled {
		experimentalCmd.GroupID = "experimental"
	}

	rootCmd.AddCommand(experimentalCmd)

	pathsCmd := newPathsCmd()
	pathsCmd.GroupID = "setup"
	rootCmd.AddCommand(pathsCmd)

	versionCmd := newVersionCmd()
	versionCmd.GroupID = "setup"
	rootCmd.AddCommand(versionCmd)

	completionCmd := newCompletionCmd()
	completionCmd.GroupID = "setup"
	rootCmd.AddCommand(completionCmd)
}

func validateAPIURL(raw string) (string, error) {
	validatedURL, err := validate.APIURL(raw)
	if err != nil {
		return "", clierrors.Wrap(clierrors.ExitConfig, "Invalid API URL", err)
	}

	return validatedURL, nil
}

// noArgs returns a Cobra positional-arg validator that rejects any arguments.
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

// VersionInfo represents version information for JSON output.
type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show version information",
		Long:    `Display the mush binary version, git commit, and build date.`,
		Example: `  mush version`,
		Args:    noArgs,
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
