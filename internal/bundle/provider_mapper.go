package bundle

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// providerMapper maps bundle assets using a YAML-driven ProviderSpec.
type providerMapper struct {
	spec *harnesstype.ProviderSpec
}

// NewProviderMapper creates an AssetMapper driven by a ProviderSpec.
func NewProviderMapper(spec *harnesstype.ProviderSpec) AssetMapper {
	return &providerMapper{spec: spec}
}

// MapAsset maps a bundle asset to the provider's native directory structure.
func (m *providerMapper) MapAsset(workDir string, layer client.BundleLayer) (string, error) {
	if err := ValidateLogicalPath(layer.LogicalPath); err != nil {
		return "", err
	}

	assets := m.spec.Assets
	if assets == nil {
		return "", fmt.Errorf("provider %s does not support bundle assets", m.spec.Name)
	}

	switch layer.AssetType {
	case "skill":
		return filepath.Join(workDir, assets.SkillDir, stripMatchingPrefix(assets.SkillDir, layer.LogicalPath)), nil
	case "agent_definition":
		return filepath.Join(workDir, assets.AgentDir, stripMatchingPrefix(assets.AgentDir, layer.LogicalPath)), nil
	case "tool_config":
		return filepath.Join(workDir, assets.ToolConfigFile), nil
	default:
		return "", fmt.Errorf("unsupported asset type for %s: %s", m.spec.Name, layer.AssetType)
	}
}

// stripMatchingPrefix removes the first path segment from logicalPath if it
// matches the last segment of dir, preventing path doubling (e.g.,
// dir=".claude/skills" + path="skills/web/SKILL.md" → "web/SKILL.md").
func stripMatchingPrefix(dir, logicalPath string) string {
	dirBase := filepath.Base(dir)

	prefix := dirBase + "/"
	if strings.HasPrefix(logicalPath, prefix) {
		return logicalPath[len(prefix):]
	}

	return logicalPath
}

// PrepareLoad creates a temp directory with assets in the provider's native structure.
// For add_dir mode harnesses, discoverable assets (skills, agents) are excluded
// from the temp dir because they are injected into the project directory instead.
func (m *providerMapper) PrepareLoad(_ context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	var skip map[string]bool
	if m.spec.BundleDir != nil && m.spec.BundleDir.Mode == "add_dir" {
		skip = discoveredAssetTypes
	}

	return prepareLoadCommon(m, cachePath, manifest, skip)
}
