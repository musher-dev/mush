//go:build unix

package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

const (
	defaultRunnerConfigRefreshSeconds = 300
	minRunnerConfigRefreshSeconds     = 60
	maxRunnerConfigRefreshSeconds     = 900
	tokenExpirySkew                   = 30 * time.Second
)

type claudeMCPConfig struct {
	MCPServers map[string]claudeMCPServer `json:"mcpServers"`
}

type claudeMCPServer struct {
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

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

// BuildMCPProviderSpecs builds provider specs from a RunnerConfigResponse, filtering
// by status, MCP flag, token validity, and expiry.
func BuildMCPProviderSpecs(cfg *client.RunnerConfigResponse, now time.Time) []MCPProviderSpec {
	if cfg == nil || len(cfg.Providers) == 0 {
		return nil
	}

	providers := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		providers = append(providers, name)
	}

	sort.Strings(providers)

	specs := make([]MCPProviderSpec, 0, len(providers))
	for _, name := range providers {
		providerConfig := cfg.Providers[name]
		if !providerConfig.Flags.MCP || providerConfig.MCP == nil || providerConfig.Credential == nil {
			continue
		}

		if providerConfig.Status != "" && !strings.EqualFold(providerConfig.Status, "active") {
			continue
		}

		if providerConfig.MCP.URL == "" || providerConfig.Credential.AccessToken == "" {
			continue
		}

		if providerConfig.Credential.ExpiresAt != nil && now.Add(tokenExpirySkew).After(*providerConfig.Credential.ExpiresAt) {
			continue
		}

		tokenType := strings.TrimSpace(providerConfig.Credential.TokenType)
		if tokenType == "" {
			tokenType = "bearer"
		}

		expiresAt := ""
		if providerConfig.Credential.ExpiresAt != nil {
			expiresAt = providerConfig.Credential.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}

		specs = append(specs, MCPProviderSpec{
			Name:      name,
			URL:       providerConfig.MCP.URL,
			TokenType: strings.ToLower(tokenType),
			Token:     providerConfig.Credential.AccessToken,
			ExpiresAt: expiresAt,
		})
	}

	return specs
}

// MCPSignature computes a SHA256 hash of the provider specs for change detection.
func MCPSignature(specs []MCPProviderSpec) (string, error) {
	if len(specs) == 0 {
		return "", nil
	}

	encoded, err := json.Marshal(specs)
	if err != nil {
		return "", fmt.Errorf("marshal mcp provider specs: %w", err)
	}

	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:]), nil
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
		authScheme := "Bearer"
		if strings.EqualFold(spec.TokenType, "basic") {
			authScheme = "Basic"
		}

		cfg.MCPServers[spec.Name] = claudeMCPServer{
			Type: "http",
			URL:  spec.URL,
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("%s %s", authScheme, spec.Token),
			},
		}
	}

	encodedConfig, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal claude mcp config: %w", err)
	}

	return encodedConfig, nil
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

		authScheme := "Bearer"
		if strings.EqualFold(spec.TokenType, "basic") {
			authScheme = "Basic"
		}

		fmt.Fprintf(&b, "[mcp_servers.%s]\n", spec.Name)
		fmt.Fprintf(&b, "type = \"http\"\n")
		fmt.Fprintf(&b, "url = %q\n", spec.URL)
		fmt.Fprintf(&b, "\n[mcp_servers.%s.http_headers]\n", spec.Name)
		fmt.Fprintf(&b, "Authorization = %q\n", fmt.Sprintf("%s %s", authScheme, spec.Token))
	}

	return []byte(b.String()), nil
}

// CreateMCPConfigFile creates an ephemeral MCP config file using the given MCPSpec.
func CreateMCPConfigFile(mcpSpec *MCPSpec, cfg *client.RunnerConfigResponse, now time.Time) (path, sig string, cleanup func() error, err error) {
	specs := BuildMCPProviderSpecs(cfg, now)
	slog.Default().Info(
		"MCP specs built",
		slog.String("component", "mcp"),
		slog.String("event.type", "mcp.specs.built"),
		slog.Int("mcp.server_count", len(specs)),
	)

	signature, signErr := MCPSignature(specs)
	if signErr != nil {
		return "", "", nil, signErr
	}

	content, buildErr := mcpSpec.BuildConfig(specs)
	if buildErr != nil {
		return "", "", nil, buildErr
	}

	if len(content) == 0 {
		return "", signature, nil, nil
	}

	// Determine file extension from MCP def format.
	ext := ".json"
	if mcpSpec.Def != nil && mcpSpec.Def.Format == "toml" {
		ext = ".toml"
	}

	file, createErr := os.CreateTemp("", "mush-mcp-*"+ext)
	if createErr != nil {
		return "", "", nil, fmt.Errorf("failed to create mcp config file: %w", createErr)
	}

	path = file.Name()
	slog.Default().Info(
		"MCP config file created",
		slog.String("component", "mcp"),
		slog.String("event.type", "mcp.config.file.created"),
		slog.String("mcp.config.path", path),
	)

	if _, writeErr := file.Write(content); writeErr != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return "", "", nil, fmt.Errorf("failed to write mcp config file: %w", writeErr)
	}

	if chmodErr := file.Chmod(0o600); chmodErr != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return "", "", nil, fmt.Errorf("failed to set mcp config permissions: %w", chmodErr)
	}

	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(path)
		return "", "", nil, fmt.Errorf("failed to close mcp config file: %w", closeErr)
	}

	cleanup = func() error {
		if path == "" {
			return nil
		}

		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove mcp config file: %w", removeErr)
		}

		slog.Default().Debug(
			"MCP config file removed",
			slog.String("component", "mcp"),
			slog.String("event.type", "mcp.config.file.removed"),
			slog.String("mcp.config.path", path),
		)

		return nil
	}

	return path, signature, cleanup, nil
}

// LoadedMCPProviderNames returns the names of providers from a RunnerConfig that
// pass all MCP filters.
func LoadedMCPProviderNames(cfg *client.RunnerConfigResponse, now time.Time) []string {
	specs := BuildMCPProviderSpecs(cfg, now)
	if len(specs) == 0 {
		return nil
	}

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}

	sort.Strings(names) // defensive: BuildMCPProviderSpecs already sorts, but guard against future changes

	return names
}
