//go:build unix

// Package harness provides a PTY-based TUI for embedding Claude Code.
//
// The harness creates a "window-in-window" interface where Claude Code runs
// interactively via PTY, with a status bar showing connection state, job info,
// and queue metrics.
//
// This implementation uses ANSI scroll regions (DECSTBM) to reserve the top
// lines for status while giving Claude Code full control of the remaining
// terminal space.
package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/term"

	"github.com/musher-dev/mush/internal/client"
)

// Config holds configuration for the harness.
type Config struct {
	Client             *client.Client
	HabitatID          string
	QueueID            string
	SupportedHarnesses []string
	InstanceID         string
	RunnerConfig       *client.RunnerConfigResponse
	TranscriptEnabled  bool
	TranscriptDir      string
	TranscriptLines    int

	// ForceSidebar skips the LR margin probe and assumes sidebar support.
	ForceSidebar bool

	// BundleLoadMode runs a single interactive session instead of polling for jobs.
	BundleLoadMode bool
	BundleName     string // for status bar display
	BundleVer      string // for status bar display
	BundleDir      string // temp dir with harness-native asset structure
	BundleSummary  BundleSummary
}

// BundleSummary captures loaded bundle component names for sidebar rendering.
type BundleSummary struct {
	Name        string
	Version     string
	TotalLayers int
	Skills      []string
	Agents      []string
	ToolConfigs []string
	Other       []string
}

// Run starts the harness TUI.
func Run(ctx context.Context, cfg *Config) error {
	// Verify we're running in a TTY
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("harness requires a terminal (TTY)")
	}

	// Create and run the harness
	model := NewRootModel(ctx, cfg)

	return model.Run()
}

// LoadedMCPServers returns the names of MCP providers that are effectively loaded.
func LoadedMCPServers(cfg *client.RunnerConfigResponse, now time.Time) []string {
	return LoadedMCPProviderNames(cfg, now)
}

// SummarizeBundleManifest builds a bundle summary for TUI chrome/sidebar use.
func SummarizeBundleManifest(manifest *client.BundleManifest) BundleSummary {
	summary := BundleSummary{}
	if manifest == nil {
		return summary
	}

	appendName := func(dst []string, logicalPath string) []string {
		name := filepath.Base(logicalPath)
		if name == "." || name == "/" || name == "" {
			name = logicalPath
		}

		return append(dst, name)
	}

	summary.TotalLayers = len(manifest.Layers)
	for _, layer := range manifest.Layers {
		switch layer.AssetType {
		case "skill":
			summary.Skills = appendName(summary.Skills, layer.LogicalPath)
		case "agent_definition":
			summary.Agents = appendName(summary.Agents, layer.LogicalPath)
		case "tool_config":
			summary.ToolConfigs = appendName(summary.ToolConfigs, layer.LogicalPath)
		default:
			summary.Other = appendName(summary.Other, layer.LogicalPath)
		}
	}

	sort.Strings(summary.Skills)
	sort.Strings(summary.Agents)
	sort.Strings(summary.ToolConfigs)
	sort.Strings(summary.Other)

	return summary
}
