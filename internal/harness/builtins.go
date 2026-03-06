//go:build unix

package harness

import (
	"github.com/musher-dev/mush/internal/harness/harnesstype"
	"github.com/musher-dev/mush/internal/harness/providers/claude"
	"github.com/musher-dev/mush/internal/harness/providers/codex"
	"github.com/musher-dev/mush/internal/harness/providers/copilot"
	"github.com/musher-dev/mush/internal/harness/providers/cursor"
	"github.com/musher-dev/mush/internal/harness/providers/gemini"
	"github.com/musher-dev/mush/internal/harness/providers/opencode"
)

// builtins lists all built-in harness provider modules.
var builtins = []harnesstype.Module{
	claude.Module,
	codex.Module,
	copilot.Module,
	cursor.Module,
	gemini.Module,
	opencode.Module,
}

func init() {
	for _, mod := range builtins {
		Register(Info{
			Name:      mod.Spec.Name,
			Available: harnesstype.AvailableFunc(mod.Spec),
			New:       mod.NewExecutor,
			MCPSpec:   mod.MCPSpec,
		})

		registerProviderSpec(mod.Spec)
	}
}
