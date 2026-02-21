//go:build unix

package harness

// MCPSpec associates a provider's MCP definition with a config builder function.
type MCPSpec struct {
	Def         *MCPDef
	BuildConfig func(specs []MCPProviderSpec) ([]byte, error)
}
