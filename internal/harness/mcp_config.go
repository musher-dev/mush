//go:build unix

package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type mcpProviderSpec struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	TokenType string `json:"tokenType"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt,omitempty"`
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

func buildMCPProviderSpecs(cfg *client.RunnerConfigResponse, now time.Time) []mcpProviderSpec {
	if cfg == nil || len(cfg.Providers) == 0 {
		return nil
	}

	providers := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		providers = append(providers, name)
	}

	sort.Strings(providers)

	specs := make([]mcpProviderSpec, 0, len(providers))
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

		specs = append(specs, mcpProviderSpec{
			Name:      name,
			URL:       providerConfig.MCP.URL,
			TokenType: strings.ToLower(tokenType),
			Token:     providerConfig.Credential.AccessToken,
			ExpiresAt: expiresAt,
		})
	}

	return specs
}

func mcpSignature(specs []mcpProviderSpec) (string, error) {
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

func buildClaudeMCPConfig(specs []mcpProviderSpec) ([]byte, error) {
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

func createClaudeMCPConfigFile(cfg *client.RunnerConfigResponse, now time.Time) (path, sig string, cleanup func() error, err error) {
	specs := buildMCPProviderSpecs(cfg, now)

	signature, err := mcpSignature(specs)
	if err != nil {
		return "", "", nil, err
	}

	content, err := buildClaudeMCPConfig(specs)
	if err != nil {
		return "", "", nil, err
	}

	if len(content) == 0 {
		return "", signature, nil, nil
	}

	file, err := os.CreateTemp("", "mush-mcp-*.json")
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to create mcp config file: %w", err)
	}

	path = file.Name()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return "", "", nil, fmt.Errorf("failed to write mcp config file: %w", err)
	}

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return "", "", nil, fmt.Errorf("failed to set mcp config permissions: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", nil, fmt.Errorf("failed to close mcp config file: %w", err)
	}

	cleanup = func() error {
		if path == "" {
			return nil
		}

		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove mcp config file: %w", err)
		}

		return nil
	}

	return path, signature, cleanup, nil
}

func loadedMCPProviderNames(cfg *client.RunnerConfigResponse, now time.Time) []string {
	specs := buildMCPProviderSpecs(cfg, now)
	if len(specs) == 0 {
		return nil
	}

	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}

	sort.Strings(names) // defensive: buildMCPProviderSpecs already sorts, but guard against future changes

	return names
}
