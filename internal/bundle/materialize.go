package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/musher-dev/mush/internal/client"
)

// InstallFromCache installs bundle assets into workDir using mapper rules.
// It performs merge semantics for tool_config assets.
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
	installed := map[string]struct{}{}

	for _, asset := range assets {
		switch asset.layer.AssetType {
		case "tool_config":
			toolConfigs[asset.targetPath] = append(toolConfigs[asset.targetPath], asset.data)
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

	paths := make([]string, 0, len(installed))
	for p := range installed {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	return paths, nil
}

// discoveredAssetTypes are asset types that harnesses discover from the project
// directory (CWD) rather than from --add-dir or similar flags. These must be
// injected into the project dir during bundle load so the harness finds them.
var discoveredAssetTypes = map[string]bool{
	"agent_definition": true,
	"skill":            true,
}

// InjectAssetsForLoad copies discoverable assets (agents, skills) from cache
// into the project directory so the harness discovers them. Tool configs are
// excluded because they are handled separately via merge logic and --mcp-config.
// It skips files that already exist (protecting user's own assets). Returns the
// list of injected paths, any validation warnings (e.g. invalid YAML frontmatter
// in SKILL.md files), and a cleanup function that removes only the files and
// directories it created.
// On error the returned cleanup removes any files and directories already created.
func InjectAssetsForLoad(
	projectDir, cachePath string,
	manifest *client.BundleManifest,
	mapper AssetMapper,
) (injected, warnings []string, cleanup func(), err error) {
	var createdFiles []string

	var createdDirs []string

	makeCleanup := func() func() {
		return func() {
			for _, f := range createdFiles {
				_ = os.Remove(f)
			}

			// Remove created directories in reverse order (deepest first).
			for i := len(createdDirs) - 1; i >= 0; i-- {
				_ = os.Remove(createdDirs[i]) // only removes if empty
			}
		}
	}

	for _, layer := range manifest.Layers {
		if !discoveredAssetTypes[layer.AssetType] {
			continue
		}

		targetPath, mapErr := mapper.MapAsset(projectDir, layer)
		if mapErr != nil {
			return nil, nil, makeCleanup(), fmt.Errorf("map asset %s: %w", layer.LogicalPath, mapErr)
		}

		// Skip if the file already exists (don't overwrite user's assets).
		if _, statErr := os.Stat(targetPath); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return nil, nil, makeCleanup(), fmt.Errorf("stat target asset %s: %w", layer.LogicalPath, statErr)
		}

		srcPath := filepath.Join(cachePath, "assets", layer.LogicalPath)

		data, readErr := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled cache
		if readErr != nil {
			return nil, nil, makeCleanup(), fmt.Errorf("read cached asset %s: %w", layer.LogicalPath, readErr)
		}

		// Validate YAML frontmatter for skill assets.
		if layer.AssetType == "skill" {
			if fmErr := ValidateSkillFrontmatter(data); fmErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", layer.LogicalPath, fmErr))
			}
		}

		// Track directories we create (deepest first for cleanup).
		dir := filepath.Dir(targetPath)

		for ancestor := dir; ancestor != projectDir; {
			if _, statErr := os.Stat(ancestor); statErr != nil {
				createdDirs = append(createdDirs, ancestor)
			} else {
				break
			}

			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				break
			}

			ancestor = parent
		}

		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil { //nolint:gosec // G301: project dir
			return nil, nil, makeCleanup(), fmt.Errorf("create directory for %s: %w", layer.LogicalPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, data, 0o644); writeErr != nil { //nolint:gosec // G306: project file
			return nil, nil, makeCleanup(), fmt.Errorf("write %s: %w", layer.LogicalPath, writeErr)
		}

		createdFiles = append(createdFiles, targetPath)

		relPath, _ := filepath.Rel(projectDir, targetPath)
		if relPath == "" {
			relPath = targetPath
		}

		injected = append(injected, relPath)
	}

	return injected, warnings, makeCleanup(), nil
}

// InjectToolConfigsForLoad merges tool_config assets from cache and writes
// them into the project directory. This is needed for harnesses that read
// their tool config from CWD only (e.g. Codex reads .codex/config.toml from
// CWD, not from --add-dir paths). Any existing file at the target path is
// backed up and restored by the returned cleanup function.
func InjectToolConfigsForLoad(
	projectDir, cachePath string,
	manifest *client.BundleManifest,
	mapper AssetMapper,
) (injected []string, cleanup func(), err error) {
	type backup struct {
		path string
		data []byte // nil means the file did not exist
	}

	var backups []backup

	var createdFiles []string

	makeCleanup := func() func() {
		return func() {
			// Remove files we created.
			for _, f := range createdFiles {
				_ = os.Remove(f)
			}

			// Restore backups.
			for _, b := range backups {
				if b.data == nil {
					_ = os.Remove(b.path)
				} else {
					_ = os.WriteFile(b.path, b.data, 0o644) //nolint:gosec // G306: restoring original
				}
			}
		}
	}

	// Collect tool_config layers grouped by target path.
	toolConfigs := map[string][][]byte{}

	for _, layer := range manifest.Layers {
		if layer.AssetType != "tool_config" {
			continue
		}

		targetPath, mapErr := mapper.MapAsset(projectDir, layer)
		if mapErr != nil {
			return nil, makeCleanup(), fmt.Errorf("map asset %s: %w", layer.LogicalPath, mapErr)
		}

		srcPath := filepath.Join(cachePath, "assets", layer.LogicalPath)

		data, readErr := os.ReadFile(srcPath) //nolint:gosec // G304: path from controlled cache
		if readErr != nil {
			return nil, makeCleanup(), fmt.Errorf("read cached asset %s: %w", layer.LogicalPath, readErr)
		}

		toolConfigs[targetPath] = append(toolConfigs[targetPath], data)
	}

	for targetPath, docs := range toolConfigs {
		// Back up existing file if present.
		var existing []byte

		data, readErr := os.ReadFile(targetPath) //nolint:gosec // G304: mapped project path

		switch {
		case readErr == nil:
			existing = data
			backups = append(backups, backup{path: targetPath, data: data})
		case os.IsNotExist(readErr):
			backups = append(backups, backup{path: targetPath, data: nil})
		default:
			return nil, makeCleanup(), fmt.Errorf("backup existing tool config %s: %w", targetPath, readErr)
		}

		merged, mergeErr := mergeToolConfigDocuments(existing, docs, targetPath)
		if mergeErr != nil {
			return nil, makeCleanup(), mergeErr
		}

		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // G301: project dir
			return nil, makeCleanup(), fmt.Errorf("create directory for %s: %w", targetPath, mkErr)
		}

		if writeErr := os.WriteFile(targetPath, merged, 0o644); writeErr != nil { //nolint:gosec // G306: project file
			return nil, makeCleanup(), fmt.Errorf("write tool config %s: %w", targetPath, writeErr)
		}

		createdFiles = append(createdFiles, targetPath)

		relPath, _ := filepath.Rel(projectDir, targetPath)
		if relPath == "" {
			relPath = targetPath
		}

		injected = append(injected, relPath)
	}

	sort.Strings(injected)

	return injected, makeCleanup(), nil
}

// InstallConflictError is returned when installation would overwrite an existing file.
type InstallConflictError struct {
	Path string
}

func (e *InstallConflictError) Error() string {
	return fmt.Sprintf("install conflict: %s", e.Path)
}
