package bundle

import (
	"context"
	"fmt"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// LoadSession describes how an ephemeral bundle load should be launched.
type LoadSession struct {
	BundleDir  string
	WorkingDir string
	Env        []string
	Prepared   []string
	Warnings   []string
	cleanup    func()
}

// Cleanup releases any temporary files created for the session.
func (s *LoadSession) Cleanup() {
	if s != nil && s.cleanup != nil {
		s.cleanup()
	}
}

// PrepareLoadSession builds a harness-specific ephemeral load session.
func PrepareLoadSession(
	ctx context.Context,
	projectDir string,
	cachePath string,
	manifest *client.BundleManifest,
	spec *harnesstype.ProviderSpec,
	mapper AssetMapper,
) (*LoadSession, error) {
	if spec == nil {
		return nil, fmt.Errorf("provider spec is required")
	}

	session := &LoadSession{
		WorkingDir: projectDir,
	}

	mode := "cwd"
	if spec.BundleDir != nil && spec.BundleDir.Mode != "" {
		mode = spec.BundleDir.Mode
	}

	switch mode {
	case "add_dir", "cd_flag":
		tmpDir, cleanup, err := mapper.PrepareLoad(ctx, cachePath, manifest)
		if err != nil {
			return nil, fmt.Errorf("prepare load: %w", err)
		}

		session.BundleDir = tmpDir
		session.cleanup = cleanup

		if hasToolConfig(manifest) && (spec.CLI == nil || spec.CLI.MCPConfig == "") {
			session.Warnings = append(session.Warnings,
				fmt.Sprintf("%s may ignore bundled tool config during ephemeral load because it does not expose an MCP/tool-config flag", spec.DisplayName))
		}

		return session, nil
	case "cwd":
		prepared, warnings, cleanup, err := InjectAssetsForLoad(projectDir, cachePath, manifest, mapper)
		if err != nil {
			return nil, err
		}

		session.BundleDir = projectDir
		session.Prepared = append(session.Prepared, prepared...)
		session.Warnings = append(session.Warnings, warnings...)
		session.cleanup = cleanup

		toolPrepared, toolCleanup, toolErr := InjectToolConfigsForLoad(projectDir, cachePath, manifest, mapper)
		if toolErr != nil {
			session.Cleanup()
			return nil, toolErr
		}

		session.Prepared = append(session.Prepared, toolPrepared...)
		session.cleanup = chainCleanup(session.cleanup, toolCleanup)

		return session, nil
	default:
		return nil, fmt.Errorf("unsupported bundle load mode %q for %s", mode, spec.Name)
	}
}

func chainCleanup(cleanups ...func()) func() {
	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				cleanups[i]()
			}
		}
	}
}

func hasToolConfig(manifest *client.BundleManifest) bool {
	if manifest == nil {
		return false
	}

	for _, layer := range manifest.Layers {
		if layer.AssetType == "tool_config" {
			return true
		}
	}

	return false
}
