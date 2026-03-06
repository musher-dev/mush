//go:build unix

package gemini

import (
	_ "embed"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

//go:embed spec.yaml
var specData []byte

var spec = harnesstype.MustParseSpec(specData)

// Module is the gemini provider module for harness registration.
var Module = harnesstype.Module{
	Spec:        spec,
	NewExecutor: func() harnesstype.Executor { return &Executor{} },
	MCPSpec: &harnesstype.MCPSpec{
		Def:         spec.MCP,
		BuildConfig: harnesstype.BuildGeminiMCPConfig,
	},
}
