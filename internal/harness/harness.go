//go:build unix

// Package harness provides the interactive watch runtime for harness executors.
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
	"github.com/musher-dev/mush/internal/harness/harnesstype"
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
	BundleWorkDir  string // working directory for interactive bundle sessions
	BundleEnv      []string
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

	return runEmbeddedHarness(ctx, cfg)
}

// LoadedMCPServers returns the names of MCP providers that are effectively loaded.
func LoadedMCPServers(cfg *client.RunnerConfigResponse, now time.Time) []string {
	return harnesstype.LoadedMCPProviderNames(cfg, now)
}

// genericDirNames are directory names that are too generic to use as display
// names for bundle assets. When a SKILL.md or AGENT.md file is nested inside
// one of these directories, we walk further up the path to find a descriptive
// ancestor directory name instead.
var genericDirNames = map[string]bool{
	"skills":  true,
	"agents":  true,
	"tools":   true,
	".claude": true,
	".gemini": true,
	".codex":  true,
}

// descriptiveAncestor walks up the directory components of logicalPath looking
// for a directory name that is not in genericDirNames. Returns fallback if no
// descriptive ancestor is found (e.g. the path is just "SKILL.md").
func descriptiveAncestor(logicalPath, fallback string) string {
	dir := filepath.Dir(logicalPath)

	for dir != "." && dir != "/" && dir != "" {
		name := filepath.Base(dir)
		if !genericDirNames[name] {
			return name
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return fallback
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

		// For conventional filenames (SKILL.md, AGENT.md), walk up the path
		// to find a descriptive directory name (e.g. "hello" instead of
		// "SKILL.md" for "skills/hello/SKILL.md" or ".claude/skills/hello/SKILL.md").
		if name == "SKILL.md" || name == "AGENT.md" {
			name = descriptiveAncestor(logicalPath, name)
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
