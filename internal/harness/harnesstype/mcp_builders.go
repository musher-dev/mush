//go:build unix

package harnesstype

import (
	"encoding/json"
	"fmt"
	"strings"
)

type claudeMCPConfig struct {
	MCPServers map[string]claudeMCPServer `json:"mcpServers"`
}

type openCodeConfig struct {
	Schema string                     `json:"$schema,omitempty"`
	MCP    map[string]openCodeMCPNode `json:"mcp,omitempty"`
}

type geminiConfig struct {
	MCPServers map[string]geminiMCPServer `json:"mcpServers,omitempty"`
}

type cursorConfig struct {
	MCPServers map[string]cursorMCPServer `json:"mcpServers,omitempty"`
}

type claudeMCPServer struct {
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type openCodeMCPNode struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Enabled bool              `json:"enabled"`
	Headers map[string]string `json:"headers,omitempty"`
}

type geminiMCPServer struct {
	HTTPURL string            `json:"httpUrl,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type cursorMCPServer struct {
	Type        string            `json:"type,omitempty"`
	URL         string            `json:"url,omitempty"`
	HTTPHeaders map[string]string `json:"httpHeaders,omitempty"`
}

func authorizationValue(tokenType, token string) string {
	authScheme := "Bearer"
	if strings.EqualFold(tokenType, "basic") {
		authScheme = "Basic"
	}

	return fmt.Sprintf("%s %s", authScheme, token)
}

func marshalMCPConfig(cfg any, providerName string) ([]byte, error) {
	encodedConfig, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s mcp config: %w", providerName, err)
	}

	return encodedConfig, nil
}

// BuildJSONMCPConfig builds a Claude-format JSON MCP config from provider specs.
func BuildJSONMCPConfig(specs []MCPProviderSpec) ([]byte, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	cfg := claudeMCPConfig{
		MCPServers: make(map[string]claudeMCPServer, len(specs)),
	}
	for _, spec := range specs {
		cfg.MCPServers[spec.Name] = claudeMCPServer{
			Type: "http",
			URL:  spec.URL,
			Headers: map[string]string{
				"Authorization": authorizationValue(spec.TokenType, spec.Token),
			},
		}
	}

	return marshalMCPConfig(cfg, "claude")
}

// BuildOpenCodeMCPConfig builds an OpenCode JSON config with MCP providers.
func BuildOpenCodeMCPConfig(specs []MCPProviderSpec) ([]byte, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	cfg := openCodeConfig{
		Schema: "https://opencode.ai/config.json",
		MCP:    make(map[string]openCodeMCPNode, len(specs)),
	}

	for _, spec := range specs {
		cfg.MCP[spec.Name] = openCodeMCPNode{
			Type:    "remote",
			URL:     spec.URL,
			Enabled: true,
			Headers: map[string]string{
				"Authorization": authorizationValue(spec.TokenType, spec.Token),
			},
		}
	}

	return marshalMCPConfig(cfg, "opencode")
}

// BuildGeminiMCPConfig builds a Gemini settings.json-compatible MCP config.
func BuildGeminiMCPConfig(specs []MCPProviderSpec) ([]byte, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	cfg := geminiConfig{
		MCPServers: make(map[string]geminiMCPServer, len(specs)),
	}

	for _, spec := range specs {
		cfg.MCPServers[spec.Name] = geminiMCPServer{
			HTTPURL: spec.URL,
			Headers: map[string]string{
				"Authorization": authorizationValue(spec.TokenType, spec.Token),
			},
		}
	}

	return marshalMCPConfig(cfg, "gemini")
}

// BuildCursorMCPConfig builds a Cursor agent.json-compatible MCP config.
func BuildCursorMCPConfig(specs []MCPProviderSpec) ([]byte, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	cfg := cursorConfig{
		MCPServers: make(map[string]cursorMCPServer, len(specs)),
	}

	for _, spec := range specs {
		cfg.MCPServers[spec.Name] = cursorMCPServer{
			Type: "http",
			URL:  spec.URL,
			HTTPHeaders: map[string]string{
				"Authorization": authorizationValue(spec.TokenType, spec.Token),
			},
		}
	}

	return marshalMCPConfig(cfg, "cursor")
}

// BuildTOMLMCPConfig builds a Codex-format TOML MCP config from provider specs.
func BuildTOMLMCPConfig(specs []MCPProviderSpec) ([]byte, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	var b strings.Builder

	for i, spec := range specs {
		if i > 0 {
			b.WriteString("\n")
		}

		fmt.Fprintf(&b, "[mcp_servers.%s]\n", spec.Name)
		fmt.Fprintf(&b, "type = \"http\"\n")
		fmt.Fprintf(&b, "url = %q\n", spec.URL)
		fmt.Fprintf(&b, "\n[mcp_servers.%s.http_headers]\n", spec.Name)
		fmt.Fprintf(&b, "Authorization = %q\n", authorizationValue(spec.TokenType, spec.Token))
	}

	return []byte(b.String()), nil
}
