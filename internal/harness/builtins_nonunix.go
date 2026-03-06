//go:build !unix

package harness

import (
	"github.com/musher-dev/mush/internal/harness/providers/claude"
	"github.com/musher-dev/mush/internal/harness/providers/codex"
	"github.com/musher-dev/mush/internal/harness/providers/copilot"
	"github.com/musher-dev/mush/internal/harness/providers/cursor"
	"github.com/musher-dev/mush/internal/harness/providers/gemini"
	"github.com/musher-dev/mush/internal/harness/providers/opencode"
)

func init() {
	registerProviderSpec(claude.Module.Spec)
	registerProviderSpec(codex.Module.Spec)
	registerProviderSpec(copilot.Module.Spec)
	registerProviderSpec(cursor.Module.Spec)
	registerProviderSpec(gemini.Module.Spec)
	registerProviderSpec(opencode.Module.Spec)
}
