package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/harness"
)

func TestPrepareLoadSession_AddDirHarnessDoesNotTouchProjectDir(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	skillDigest := storeTestAsset(t, "skill")
	agentDigest := storeTestAsset(t, "agent")
	toolDigest := storeTestAsset(t, `{"mcpServers":{}}`)

	spec, ok := harness.GetProvider("claude")
	if !ok {
		t.Fatal("claude provider not found")
	}

	session, err := PrepareLoadSession(
		t.Context(),
		projectDir,
		&client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "skills/web/SKILL.md", AssetType: "skill", ContentSHA256: skillDigest},
				{LogicalPath: "researcher.md", AssetType: "agent_definition", ContentSHA256: agentDigest},
				{LogicalPath: "mcp.json", AssetType: "tool_config", ContentSHA256: toolDigest},
			},
		},
		spec,
		NewProviderMapper(spec),
	)
	if err != nil {
		t.Fatalf("PrepareLoadSession() error = %v", err)
	}

	// Register cleanup early so temp dirs are removed even if assertions fail.
	cleanupCalled := false

	t.Cleanup(func() {
		if !cleanupCalled {
			session.Cleanup()
		}
	})

	if session.BundleDir == projectDir {
		t.Fatal("add_dir harness should use an external bundle directory")
	}

	if session.WorkingDir != projectDir {
		t.Fatalf("WorkingDir = %q, want %q", session.WorkingDir, projectDir)
	}

	// Agent should be injected into project dir (Claude Code only discovers agents from CWD).
	agentPath := filepath.Join(projectDir, ".claude", "agents", "researcher.md")
	if _, statErr := os.Stat(agentPath); statErr != nil {
		t.Fatalf("expected agent injected into project dir: %v", statErr)
	}

	// Skill should NOT be in project dir (stays in temp dir, discovered via --add-dir).
	if _, statErr := os.Stat(filepath.Join(projectDir, ".claude", "skills", "web", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Fatal("skill should not be injected into project dir for add_dir harness")
	}

	if _, statErr := os.Stat(filepath.Join(session.BundleDir, ".claude", "skills", "web", "SKILL.md")); statErr != nil {
		t.Fatalf("expected skill in external bundle dir: %v", statErr)
	}

	// session.Prepared should contain the agent path.
	found := false

	for _, p := range session.Prepared {
		if filepath.Base(p) == "researcher.md" {
			found = true

			break
		}
	}

	if !found {
		t.Fatalf("session.Prepared should contain agent path, got %v", session.Prepared)
	}

	// Cleanup should remove the agent from project dir.
	cleanupCalled = true

	session.Cleanup()

	if _, statErr := os.Stat(agentPath); !os.IsNotExist(statErr) {
		t.Fatalf("cleanup should remove injected agent from project dir; stat err = %v", statErr)
	}
}

func TestPrepareLoadSession_CWDHarnessFallsBackToProjectInjection(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	projectDir := t.TempDir()

	commandDigest := storeTestAsset(t, "prompt = \"review\"")
	toolDigest := storeTestAsset(t, `{"theme":"test"}`)

	spec, ok := harness.GetProvider("gemini")
	if !ok {
		t.Fatal("gemini provider not found")
	}

	session, err := PrepareLoadSession(
		t.Context(),
		projectDir,
		&client.BundleManifest{
			Layers: []client.BundleLayer{
				{LogicalPath: "commands/review.toml", AssetType: "skill", ContentSHA256: commandDigest},
				{LogicalPath: "tools/settings.json", AssetType: "tool_config", ContentSHA256: toolDigest},
			},
		},
		spec,
		NewProviderMapper(spec),
	)
	if err != nil {
		t.Fatalf("PrepareLoadSession() error = %v", err)
	}

	defer session.Cleanup()

	if session.BundleDir != projectDir {
		t.Fatalf("BundleDir = %q, want project dir %q", session.BundleDir, projectDir)
	}

	if _, statErr := os.Stat(filepath.Join(projectDir, ".gemini", "commands", "review.toml")); statErr != nil {
		t.Fatalf("expected injected skill in project dir: %v", statErr)
	}

	if _, statErr := os.Stat(filepath.Join(projectDir, ".gemini", "settings.json")); statErr != nil {
		t.Fatalf("expected injected tool config in project dir: %v", statErr)
	}
}
