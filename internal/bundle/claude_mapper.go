package bundle

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
)

// ClaudeAssetMapper maps bundle assets to Claude Code's native directory structure.
type ClaudeAssetMapper struct{}

// MapAsset maps a bundle asset to the Claude Code directory structure.
//
// Asset type mapping:
//   - "skill"            → .claude/skills/<logical_path>
//   - "agent_definition" → .claude/agents/<logical_path>
//   - "tool_config"      → .mcp.json (merged)
func (m *ClaudeAssetMapper) MapAsset(workDir string, layer client.BundleLayer) (string, error) {
	if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
		return "", err
	}

	switch layer.AssetType {
	case "skill":
		return filepath.Join(workDir, ".claude", "skills", layer.LogicalPath), nil
	case "agent_definition":
		return filepath.Join(workDir, ".claude", "agents", layer.LogicalPath), nil
	case "tool_config":
		return filepath.Join(workDir, ".mcp.json"), nil
	default:
		return "", fmt.Errorf("unsupported asset type for claude: %s", layer.AssetType)
	}
}

// PrepareLoad creates a temp directory with assets in Claude's native structure.
func (m *ClaudeAssetMapper) PrepareLoad(_ context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	return prepareLoadCommon(m, cachePath, manifest)
}
