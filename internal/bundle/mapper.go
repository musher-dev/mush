package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/client"
)

// AssetMapper maps bundle assets to a harness's native directory structure.
type AssetMapper interface {
	// MapAsset returns the target path for a bundle asset in the harness's native structure.
	// workDir is the project directory (e.g., current working directory).
	MapAsset(workDir string, layer client.BundleLayer) (string, error)

	// PrepareLoad creates a temp directory with assets in native structure for `bundle load`.
	// Returns the temp dir path and a cleanup function.
	PrepareLoad(ctx context.Context, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error)

	// MergesAgents returns true if agent definitions are composed into a single file
	// (e.g., AGENTS.md), false if each agent is written as an individual file.
	MergesAgents() bool
}

// prepareLoadCommon is shared logic for preparing load dirs across mappers.
func prepareLoadCommon(mapper AssetMapper, cachePath string, manifest *client.BundleManifest) (tmpDir string, cleanup func(), err error) {
	tmpDir, err = os.MkdirTemp("", "mush-bundle-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup = func() {
		_ = os.RemoveAll(tmpDir)
	}

	type mappedAsset struct {
		layer      client.BundleLayer
		targetPath string
		data       []byte
	}

	assets := make([]mappedAsset, 0, len(manifest.Layers))

	for _, layer := range manifest.Layers {
		if vErr := ValidateLogicalPath(layer.LogicalPath); vErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("invalid logical path: %w", vErr)
		}

		// Get target path via mapper.
		targetPath, mapErr := mapper.MapAsset(tmpDir, layer)
		if mapErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("map asset %s: %w", layer.LogicalPath, mapErr)
		}

		// Read from cache.
		srcPath := filepath.Join(cachePath, "assets", layer.LogicalPath)

		data, readErr := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled cache
		if readErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("read cached asset %s: %w", layer.LogicalPath, readErr)
		}

		assets = append(assets, mappedAsset{
			layer:      layer,
			targetPath: targetPath,
			data:       data,
		})
	}

	toolConfigs := map[string][][]byte{}
	agentsMD := map[string][]AgentDoc{}

	for _, asset := range assets {
		switch {
		case asset.layer.AssetType == "tool_config":
			toolConfigs[asset.targetPath] = append(toolConfigs[asset.targetPath], asset.data)
		case asset.layer.AssetType == "agent_definition" && mapper.MergesAgents():
			agentsMD[asset.targetPath] = append(agentsMD[asset.targetPath], AgentDoc{
				Name:    asset.layer.LogicalPath,
				Content: asset.data,
			})
		default:
			if mkErr := os.MkdirAll(filepath.Dir(asset.targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: temp dir
				cleanup()
				return "", nil, fmt.Errorf("create dir for %s: %w", asset.targetPath, mkErr)
			}

			if writeErr := os.WriteFile(asset.targetPath, asset.data, 0o644); writeErr != nil { //nolint:gosec // G306: temp file
				cleanup()
				return "", nil, fmt.Errorf("write %s: %w", asset.targetPath, writeErr)
			}
		}
	}

	for targetPath, docs := range toolConfigs {
		merged, mergeErr := mergeToolConfigDocuments(nil, docs, targetPath)
		if mergeErr != nil {
			cleanup()
			return "", nil, mergeErr
		}

		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: temp dir
			cleanup()
			return "", nil, fmt.Errorf("create dir for %s: %w", targetPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, merged, 0o644); writeErr != nil { //nolint:gosec // G306: temp file
			cleanup()
			return "", nil, fmt.Errorf("write merged tool config %s: %w", targetPath, writeErr)
		}
	}

	for targetPath, docs := range agentsMD {
		merged := ComposeAgentsMarkdown(nil, docs)

		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: temp dir
			cleanup()
			return "", nil, fmt.Errorf("create dir for %s: %w", targetPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, merged, 0o644); writeErr != nil { //nolint:gosec // G306: temp file
			cleanup()
			return "", nil, fmt.Errorf("write merged agents file %s: %w", targetPath, writeErr)
		}
	}

	return tmpDir, cleanup, nil
}

func mergeToolConfigDocuments(existing []byte, docs [][]byte, targetPath string) ([]byte, error) {
	switch {
	case strings.HasSuffix(targetPath, ".json"):
		merged, err := MergeJSONDocs(existing, docs)
		if err != nil {
			return nil, fmt.Errorf("merge json tool config %s: %w", targetPath, err)
		}

		return merged, nil
	case strings.HasSuffix(targetPath, ".toml"):
		merged, err := MergeTOMLDocs(existing, docs)
		if err != nil {
			return nil, fmt.Errorf("merge toml tool config %s: %w", targetPath, err)
		}

		return merged, nil
	default:
		combined := make([]byte, 0, len(existing)+1)
		combined = append(combined, existing...)

		for _, doc := range docs {
			if len(combined) > 0 && combined[len(combined)-1] != '\n' {
				combined = append(combined, '\n')
			}

			combined = append(combined, doc...)
		}

		return combined, nil
	}
}
