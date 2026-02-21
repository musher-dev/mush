package harness

// MCPSpec associates a provider's MCP definition with a config builder function.
type MCPSpec struct {
	Def         *MCPDef
	BuildConfig func(specs []MCPProviderSpec) ([]byte, error)
}

// MCPProviderSpec describes a single MCP provider for config generation.
type MCPProviderSpec struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	TokenType string `json:"tokenType"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}
