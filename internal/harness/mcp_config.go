//go:build unix

package harness

import (
	"sort"
	"time"

	harnessstate "github.com/musher-dev/mush/internal/harness/state"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

const (
	defaultRunnerConfigRefreshSeconds = 300
	minRunnerConfigRefreshSeconds     = 60
	maxRunnerConfigRefreshSeconds     = 900
)

func normalizeRefreshInterval(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultRunnerConfigRefreshSeconds
	}

	if seconds < minRunnerConfigRefreshSeconds {
		seconds = minRunnerConfigRefreshSeconds
	}

	if seconds > maxRunnerConfigRefreshSeconds {
		seconds = maxRunnerConfigRefreshSeconds
	}

	return time.Duration(seconds) * time.Second
}

// buildMCPServerStatuses builds snapshot-ready MCP server status entries from a JobLoop.
func buildMCPServerStatuses(jobs *JobLoop, now time.Time) []harnessstate.MCPServerStatus {
	cfg := jobs.RunnerConfig()
	if cfg == nil || len(cfg.Providers) == 0 {
		return nil
	}

	loadedSet := map[string]bool{}
	for _, name := range harnesstype.LoadedMCPProviderNames(cfg, now) {
		loadedSet[name] = true
	}

	names := make([]string, 0, len(cfg.Providers))

	for name, provider := range cfg.Providers {
		if !provider.Flags.MCP || provider.MCP == nil {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	statuses := make([]harnessstate.MCPServerStatus, 0, len(names))

	for _, name := range names {
		provider := cfg.Providers[name]
		authenticated := false
		expired := false

		if provider.Credential != nil {
			authenticated = provider.Credential.AccessToken != ""
			if provider.Credential.ExpiresAt != nil && !provider.Credential.ExpiresAt.After(now) {
				expired = true
				authenticated = false
			}
		}

		statuses = append(statuses, harnessstate.MCPServerStatus{
			Name:          name,
			Loaded:        loadedSet[name],
			Authenticated: authenticated,
			Expired:       expired,
		})
	}

	return statuses
}
