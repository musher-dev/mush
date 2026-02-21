package bundle

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

// providerMapper maps bundle assets using a YAML-driven ProviderSpec.
type providerMapper struct {
	spec *harness.ProviderSpec
}

// NewProviderMapper creates an AssetMapper driven by a ProviderSpec.
func NewProviderMapper(spec *harness.ProviderSpec) AssetMapper {
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
		return filepath.Join(workDir, assets.SkillDir, layer.LogicalPath), nil
	case "agent_definition":
		return filepath.Join(workDir, assets.AgentDir, layer.LogicalPath), nil
	case "tool_config":
		return filepath.Join(workDir, assets.ToolConfigFile), nil
	default:
		return "", fmt.Errorf("unsupported asset type for %s: %s", m.spec.Name, layer.AssetType)
	}
}

// PrepareLoad creates a temp directory with assets in the provider's native structure.
func (m *providerMapper) PrepareLoad(_ context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	return prepareLoadCommon(m, cachePath, manifest)
}
