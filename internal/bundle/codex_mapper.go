package bundle

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
)

// CodexAssetMapper maps bundle assets to Codex CLI's native directory structure.
type CodexAssetMapper struct{}

// MapAsset maps a bundle asset to the Codex directory structure.
//
// Asset type mapping:
//   - "skill"            → .agents/skills/<logical_path>
//   - "agent_definition" → AGENTS.md (appended)
//   - "tool_config"      → .codex/config.toml (merged)
func (m *CodexAssetMapper) MapAsset(workDir string, layer client.BundleLayer) (string, error) {
	if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
		return "", err
	}

	switch layer.AssetType {
	case "skill":
		return filepath.Join(workDir, ".agents", "skills", layer.LogicalPath), nil
	case "agent_definition":
		return filepath.Join(workDir, "AGENTS.md"), nil
	case "tool_config":
		return filepath.Join(workDir, ".codex", "config.toml"), nil
	default:
		return "", fmt.Errorf("unsupported asset type for codex: %s", layer.AssetType)
	}
}

// PrepareLoad creates a temp directory with assets in Codex's native structure.
func (m *CodexAssetMapper) PrepareLoad(_ context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	return prepareLoadCommon(m, cachePath, manifest)
}
