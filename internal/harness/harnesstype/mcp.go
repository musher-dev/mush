//go:build unix

package harnesstype

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

const tokenExpirySkew = 30 * time.Second

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

// CreateMCPConfigFile creates an ephemeral MCP config file using the given MCPSpec.
func CreateMCPConfigFile(logger *slog.Logger, mcpSpec *MCPSpec, cfg *client.RunnerConfigResponse, now time.Time) (path, sig string, cleanup func() error, err error) {
	specs := BuildMCPProviderSpecs(cfg, now)
	logger.Info(
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
	logger.Info(
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

		logger.Debug(
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
