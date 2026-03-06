package harnesstype

// Module describes a self-contained harness provider.
// Each provider directory exports a single Module variable.
type Module struct {
	// Spec is the provider's parsed YAML specification.
	Spec *ProviderSpec

	// NewExecutor creates a new Executor instance for this provider.
	NewExecutor func() Executor

	// MCPSpec describes MCP configuration support. Nil means no MCP support.
	MCPSpec *MCPSpec
}
