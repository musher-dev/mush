//go:build unix

package claude

import (
	_ "embed"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

//go:embed spec.yaml
var specData []byte

// spec is the parsed provider spec, shared by Module and executor code.
var spec = harnesstype.MustParseSpec(specData)

// mcpSpec is the MCP configuration for the claude provider.
var mcpSpec = &harnesstype.MCPSpec{
	Def:         spec.MCP,
	BuildConfig: harnesstype.BuildJSONMCPConfig,
}

// Module is the claude provider module for harness registration.
var Module = harnesstype.Module{
	Spec:        spec,
	NewExecutor: func() harnesstype.Executor { return NewExecutor() },
	MCPSpec:     mcpSpec,
}
