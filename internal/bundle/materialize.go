package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/musher-dev/mush/internal/client"
)

// InstallFromCache installs bundle assets into workDir using mapper rules.
// It performs merge semantics for tool_config and AGENTS.md agent definitions.
func InstallFromCache(
	workDir string,
	cachePath string,
	manifest *client.BundleManifest,
	mapper AssetMapper,
	force bool,
) ([]string, error) {
	type mappedAsset struct {
		layer      client.BundleLayer
		targetPath string
		data       []byte
	}

	assets := make([]mappedAsset, 0, len(manifest.Layers))

	for _, layer := range manifest.Layers {
		targetPath, mapErr := mapper.MapAsset(workDir, layer)
		if mapErr != nil {
			return nil, fmt.Errorf("map asset %s: %w", layer.LogicalPath, mapErr)
		}

		srcPath := filepath.Join(cachePath, "assets", layer.LogicalPath)

		data, readErr := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled cache
		if readErr != nil {
			return nil, fmt.Errorf("read cached asset %s: %w", layer.LogicalPath, readErr)
		}

		assets = append(assets, mappedAsset{
			layer:      layer,
			targetPath: targetPath,
			data:       data,
		})
	}

	toolConfigs := map[string][][]byte{}
	agentsMD := map[string][]AgentDoc{}
	installed := map[string]struct{}{}

	for _, asset := range assets {
		switch {
		case asset.layer.AssetType == "tool_config":
			toolConfigs[asset.targetPath] = append(toolConfigs[asset.targetPath], asset.data)
		case asset.layer.AssetType == "agent_definition" && filepath.Base(asset.targetPath) == "AGENTS.md":
			agentsMD[asset.targetPath] = append(agentsMD[asset.targetPath], AgentDoc{
				Name:    asset.layer.LogicalPath,
				Content: asset.data,
			})
		default:
			if !force {
				if _, statErr := os.Stat(asset.targetPath); statErr == nil {
					return nil, &InstallConflictError{Path: asset.targetPath}
				}
			}

			if mkErr := os.MkdirAll(filepath.Dir(asset.targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: project dir
				return nil, fmt.Errorf("create directory for %s: %w", asset.targetPath, mkErr)
			}

			if writeErr := os.WriteFile(asset.targetPath, asset.data, 0o644); writeErr != nil { //nolint:gosec // G306: project file
				return nil, fmt.Errorf("write %s: %w", asset.targetPath, writeErr)
			}

			relPath, _ := filepath.Rel(workDir, asset.targetPath)
			if relPath == "" {
				relPath = asset.targetPath
			}

			installed[relPath] = struct{}{}
		}
	}

	for targetPath, docs := range toolConfigs {
		var existing []byte
		if data, readErr := os.ReadFile(targetPath); readErr == nil { //nolint:gosec // G304: mapped project path
			existing = data
		}

		merged, mergeErr := mergeToolConfigDocuments(existing, docs, targetPath)
		if mergeErr != nil {
			return nil, mergeErr
		}

		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: project dir
			return nil, fmt.Errorf("create directory for %s: %w", targetPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, merged, 0o644); writeErr != nil { //nolint:gosec // G306: project file
			return nil, fmt.Errorf("write merged tool config %s: %w", targetPath, writeErr)
		}

		relPath, _ := filepath.Rel(workDir, targetPath)
		if relPath == "" {
			relPath = targetPath
		}

		installed[relPath] = struct{}{}
	}

	for targetPath, docs := range agentsMD {
		var existing []byte
		if data, readErr := os.ReadFile(targetPath); readErr == nil { //nolint:gosec // G304: mapped project path
			existing = data
		}

		merged := ComposeAgentsMarkdown(existing, docs)

		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: project dir
			return nil, fmt.Errorf("create directory for %s: %w", targetPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, merged, 0o644); writeErr != nil { //nolint:gosec // G306: project file
			return nil, fmt.Errorf("write merged agents file %s: %w", targetPath, writeErr)
		}

		relPath, _ := filepath.Rel(workDir, targetPath)
		if relPath == "" {
			relPath = targetPath
		}

		installed[relPath] = struct{}{}
	}

	paths := make([]string, 0, len(installed))
	for p := range installed {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	return paths, nil
}

// InstallConflictError is returned when installation would overwrite an existing file.
type InstallConflictError struct {
	Path string
}

func (e *InstallConflictError) Error() string {
	return fmt.Sprintf("install conflict: %s", e.Path)
}
