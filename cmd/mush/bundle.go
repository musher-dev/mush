//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Manage agent bundles",
		Long: `Pull versioned collections of agent assets from the Musher platform
and either load them ephemerally or install them into a harness's native
directory structure.`,
	}

	cmd.AddCommand(newBundleLoadCmd())
	cmd.AddCommand(newBundleRunCmd())
	cmd.AddCommand(newBundleInstallCmd())
	cmd.AddCommand(newBundleListCmd())
	cmd.AddCommand(newBundleInfoCmd())
	cmd.AddCommand(newBundleUninstallCmd())

	return cmd
}

func newBundleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local bundle cache and installed bundles",
		Long: `Show all bundles stored in the local cache and any bundles installed in the
current project directory.`,
		Example: `  mush bundle list`,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			cached, err := bundle.ListCached()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list cached bundles", err)
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installed, err := bundle.LoadInstalled(workDir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load installed bundles", err)
			}

			out.Println("Cached bundles:")

			if len(cached) == 0 {
				out.Print("  (none)\n")
			} else {
				for _, c := range cached {
					out.Print("  %s/%s:%s (%d assets)\n", c.Namespace, c.Slug, c.Version, c.AssetCount)
				}
			}

			out.Println()
			out.Println("Installed bundles in current project:")

			if len(installed) == 0 {
				out.Print("  (none)\n")
			} else {
				sort.Slice(installed, func(i, j int) bool {
					if installed[i].Ref != installed[j].Ref {
						return installed[i].Ref < installed[j].Ref
					}

					return installed[i].Harness < installed[j].Harness
				})

				for i := range installed {
					out.Print("  %s:%s [%s] (%d assets)\n", installed[i].Ref, installed[i].Version, installed[i].Harness, len(installed[i].Assets))
				}
			}

			return nil
		},
	}
}

func newBundleInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <namespace/slug>[:<version>]",
		Short: "Show details for a bundle reference",
		Long: `Show hub metadata, cached versions, and installation status for a bundle.

Queries the Musher Hub for bundle details (public, no auth required) and
also checks the local cache and current project directory.`,
		Example: `  mush bundle info acme/my-agent-kit
  mush bundle info acme/my-agent-kit:1.0.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			ref, err := bundle.ParseRef(strings.TrimSpace(args[0]))
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use: mush bundle info <namespace/slug>[:<version>]",
					Code:    clierrors.ExitUsage,
				}
			}

			// Query the hub (public, no auth required). Failures are non-fatal.
			cfg := config.Load()
			hubClient := client.New(cfg.APIURL(), "")
			hubDetail, hubErr := hubClient.GetHubBundleDetail(cmd.Context(), ref.Namespace, ref.Slug)

			cached, err := bundle.ListCached()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list cached bundles", err)
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			installed, err := bundle.LoadInstalled(workDir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to load installed bundles", err)
			}

			var cachedMatches []bundle.CachedBundle

			for i := range cached {
				if cached[i].Namespace != ref.Namespace || cached[i].Slug != ref.Slug {
					continue
				}

				if ref.Version != "" && cached[i].Version != ref.Version {
					continue
				}

				cachedMatches = append(cachedMatches, cached[i])
			}

			var installedMatches []bundle.InstalledBundle

			for i := range installed {
				if installed[i].Namespace != ref.Namespace || installed[i].Slug != ref.Slug {
					continue
				}

				if ref.Version != "" && installed[i].Version != ref.Version {
					continue
				}

				installedMatches = append(installedMatches, installed[i])
			}

			if hubErr != nil && len(cachedMatches) == 0 && len(installedMatches) == 0 {
				return &clierrors.CLIError{
					Message: fmt.Sprintf("Bundle not found: %s", ref.String()),
					Hint:    "Not found on the Musher Hub or locally.\nSearch for available bundles with 'mush hub search <query>'",
					Code:    clierrors.ExitGeneral,
				}
			}

			out.Print("Bundle: %s\n\n", ref.String())

			// Hub metadata section.
			if hubDetail != nil {
				printHubDetail(out, hubDetail)
			}

			// Status section.
			out.Println("Status:")

			if hubDetail != nil && hubErr == nil {
				out.Print("  Hub:       available\n")
			} else {
				out.Print("  Hub:       not found\n")
			}

			if len(cachedMatches) > 0 {
				versions := make([]string, 0, len(cachedMatches))
				for _, c := range cachedMatches {
					versions = append(versions, c.Version)
				}

				out.Print("  Cached:    %s\n", strings.Join(versions, ", "))
			} else {
				out.Print("  Cached:    no\n")
			}

			if len(installedMatches) > 0 {
				for i := range installedMatches {
					out.Print("  Installed: %s [%s]\n", installedMatches[i].Version, installedMatches[i].Harness)
				}
			} else {
				out.Print("  Installed: no\n")
			}

			return nil
		},
	}
}

// printHubDetail formats hub bundle metadata for display.
func printHubDetail(out *output.Writer, detail *client.HubBundleDetail) {
	if detail.DisplayName != "" {
		out.Print("Name:        %s\n", detail.DisplayName)
	}

	if detail.Publisher.Handle != "" {
		out.Print("Publisher:   %s\n", detail.Publisher.Handle)
	}

	if detail.Summary != "" {
		out.Print("Summary:     %s\n", detail.Summary)
	}

	if detail.LatestVersion != "" {
		out.Print("Latest:      %s\n", detail.LatestVersion)
	}

	if detail.License != "" {
		out.Print("License:     %s\n", detail.License)
	}

	if detail.StarsCount > 0 || detail.DownloadsTotal > 0 {
		out.Print("Stars: %d  Downloads: %d\n", detail.StarsCount, detail.DownloadsTotal)
	}

	if detail.IsDeprecated {
		out.Warning("This bundle is deprecated")
	}

	out.Println()
}

func newBundleUninstallCmd() *cobra.Command {
	var (
		harnessType string
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "uninstall <namespace/slug>[:<version>] --harness <type>",
		Short: "Remove installed bundle assets from the current project",
		Long: `Remove previously installed bundle assets from the current project directory.

Lists the files that will be removed and prompts for confirmation unless
--force is passed.`,
		Example: `  mush bundle uninstall acme/my-kit --harness claude
  mush bundle uninstall acme/my-kit:1.0.0 --harness claude --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			ref, err := bundle.ParseRef(strings.TrimSpace(args[0]))
			if err != nil {
				return &clierrors.CLIError{
					Message: err.Error(),
					Hint:    "Use: mush bundle uninstall <namespace/slug>[:<version>] --harness <type>",
					Code:    clierrors.ExitUsage,
				}
			}

			if harnessType == "" {
				return &clierrors.CLIError{
					Message: "Harness type is required for bundle uninstall",
					Hint:    fmt.Sprintf("Use --harness flag. Available: %s", joinNames(harness.RegisteredNames())),
					Code:    clierrors.ExitUsage,
				}
			}

			normalized, err := normalizeHarnessType(harnessType)
			if err != nil {
				return err
			}

			workDir, err := os.Getwd()
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to get working directory", err)
			}

			entry, err := bundle.FindInstalled(workDir, ref, normalized)
			if err != nil {
				if errors.Is(err, bundle.ErrNotInstalled) {
					return clierrors.BundleNotFound(ref.String())
				}

				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read installed bundles", err)
			}

			out.Println("The following files will be removed:")

			for _, relPath := range entry.Assets {
				out.Print("  %s\n", relPath)
			}

			out.Println()

			if !force {
				if out.NoInput {
					return clierrors.New(clierrors.ExitUsage, "Cannot confirm uninstall in non-interactive mode").
						WithHint("Use --force to skip confirmation")
				}

				prompter := prompt.New(out)

				confirmed, promptErr := prompter.Confirm(
					fmt.Sprintf("Uninstall %s (%s)? This will remove %d file(s)", ref.String(), normalized, len(entry.Assets)),
					false,
				)
				if promptErr != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read confirmation", promptErr)
				}

				if !confirmed {
					out.Info("Uninstall canceled")
					return nil
				}
			}

			removed, err := bundle.Uninstall(workDir, ref, normalized)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to uninstall bundle assets", err)
			}

			for _, relPath := range removed {
				out.Success("Removed: %s", relPath)
			}

			out.Println()
			out.Success("Uninstalled %s for harness %s", ref.String(), normalized)

			return nil
		},
	}

	cmd.Flags().StringVar(&harnessType, "harness", "", "Harness type to uninstall from (required)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

// mapperForHarness returns the AssetMapper for a given harness type.
func mapperForHarness(harnessType string) bundle.AssetMapper {
	spec, ok := harness.GetProvider(harnessType)
	if !ok || !harness.HasAssetMapping(harnessType) {
		return nil
	}

	return bundle.NewProviderMapper(spec)
}

// joinNames joins a slice of strings with ", ".
func joinNames(names []string) string {
	result := ""

	for i, n := range names {
		if i > 0 {
			result += ", "
		}

		result += n
	}

	return result
}

// isForbiddenError returns true if the error chain contains an HTTP 403 status.
func isForbiddenError(err error) bool {
	return strings.Contains(err.Error(), "status 403")
}

// localFlagName returns the flag name for error messages.
func localFlagName(hasDir, hasSample bool) string {
	if hasDir {
		return "dir"
	}

	if hasSample {
		return "sample"
	}

	return "local"
}
