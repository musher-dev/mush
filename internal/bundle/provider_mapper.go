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
func (m *providerMapper) MapAsset(workDir string, layer *client.BundleLayer) (string, error) {
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
	case "agent_definition", "agent_spec":
		return filepath.Join(workDir, assets.AgentDir, stripMatchingPrefix(assets.AgentDir, layer.LogicalPath)), nil
	case "tool_config":
		return filepath.Join(workDir, assets.ToolConfigFile), nil
	case "other", "config", "reference", "prompt":
		return filepath.Join(workDir, layer.LogicalPath), nil
	default:
		return "", fmt.Errorf("unsupported asset type for %s: %s", m.spec.Name, layer.AssetType)
	}
}

// stripMatchingPrefix removes the leading directory from logicalPath when it
// overlaps with dir, preventing path doubling. It handles two cases:
//
//  1. Full dir prefix: dir=".claude/skills" + path=".claude/skills/foo.md" → "foo.md"
//  2. Base-only prefix: dir=".claude/skills" + path="skills/web/SKILL.md" → "web/SKILL.md"
func stripMatchingPrefix(dir, logicalPath string) string {
	// Case 1: logicalPath starts with the full dir (e.g. hub-published assets).
	fullPrefix := dir + "/"
	if strings.HasPrefix(logicalPath, fullPrefix) {
		return logicalPath[len(fullPrefix):]
	}

	// Case 2: logicalPath starts with only the base segment of dir.
	basePrefix := filepath.Base(dir) + "/"
	if strings.HasPrefix(logicalPath, basePrefix) {
		return logicalPath[len(basePrefix):]
	}

	return logicalPath
}

// PrepareLoad creates a temp directory with assets in the provider's native structure.
func (m *providerMapper) PrepareLoad(_ context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	return prepareLoadCommon(m, cachePath, manifest, nil)
}
