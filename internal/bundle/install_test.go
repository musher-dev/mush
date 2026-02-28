package bundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrackInstallAndLoad(t *testing.T) {
	workDir := t.TempDir()

	bundle := &InstalledBundle{
		Slug:      "my-bundle",
		Version:   "1.0.0",
		Harness:   "claude",
		Assets:    []string{".claude/skills/skill.md"},
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}

	if err := TrackInstall(workDir, bundle); err != nil {
		t.Fatalf("TrackInstall() error = %v", err)
	}

	installed, err := LoadInstalled(workDir)
	if err != nil {
		t.Fatalf("LoadInstalled() error = %v", err)
	}

	if len(installed) != 1 {
		t.Fatalf("LoadInstalled() len = %d, want 1", len(installed))
	}

	got := installed[0]
	if got.Slug != bundle.Slug || got.Version != bundle.Version || got.Harness != bundle.Harness {
		t.Fatalf("LoadInstalled()[0] = %+v, want %+v", got, *bundle)
	}
}

func TestTrackInstallReplacesExisting(t *testing.T) {
	workDir := t.TempDir()

	v1 := &InstalledBundle{
		Slug:      "my-bundle",
		Version:   "1.0.0",
		Harness:   "claude",
		Assets:    []string{".claude/skills/old.md"},
		Timestamp: time.Now().UTC(),
	}
	if err := TrackInstall(workDir, v1); err != nil {
		t.Fatalf("TrackInstall v1 error = %v", err)
	}

	v2 := &InstalledBundle{
		Slug:      "my-bundle",
		Version:   "2.0.0",
		Harness:   "claude",
		Assets:    []string{".claude/skills/new.md"},
		Timestamp: time.Now().UTC(),
	}
	if err := TrackInstall(workDir, v2); err != nil {
		t.Fatalf("TrackInstall v2 error = %v", err)
	}

	installed, err := LoadInstalled(workDir)
	if err != nil {
		t.Fatalf("LoadInstalled() error = %v", err)
	}

	if len(installed) != 1 {
		t.Fatalf("LoadInstalled() len = %d, want 1 (replace, not append)", len(installed))
	}

	if installed[0].Version != "2.0.0" {
		t.Fatalf("installed version = %q, want %q", installed[0].Version, "2.0.0")
	}
}

func TestUninstallRemovesFilesAndEntry(t *testing.T) {
	workDir := t.TempDir()

	// Create the asset file.
	assetRel := filepath.Join(".claude", "skills", "skill.md")
	assetAbs := filepath.Join(workDir, assetRel)

	if err := os.MkdirAll(filepath.Dir(assetAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	if err := os.WriteFile(assetAbs, []byte("# skill"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	bundle := &InstalledBundle{
		Slug:      "my-bundle",
		Version:   "1.0.0",
		Harness:   "claude",
		Assets:    []string{assetRel},
		Timestamp: time.Now().UTC(),
	}
	if err := TrackInstall(workDir, bundle); err != nil {
		t.Fatalf("TrackInstall error = %v", err)
	}

	removed, err := Uninstall(workDir, "my-bundle", "claude")
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if len(removed) != 1 {
		t.Fatalf("Uninstall() removed len = %d, want 1", len(removed))
	}

	// Asset file should be gone.
	if _, statErr := os.Stat(assetAbs); !os.IsNotExist(statErr) {
		t.Fatalf("asset file still exists after uninstall")
	}

	// installed.json should have no entries.
	installed, err := LoadInstalled(workDir)
	if err != nil {
		t.Fatalf("LoadInstalled() error = %v", err)
	}

	if len(installed) != 0 {
		t.Fatalf("LoadInstalled() len = %d, want 0", len(installed))
	}
}

func TestSaveInstalledAtomic(t *testing.T) {
	workDir := t.TempDir()
	mushDir := filepath.Join(workDir, ".mush")

	// Write initial state.
	bundle := &InstalledBundle{
		Slug:      "test",
		Version:   "1.0.0",
		Harness:   "bash",
		Assets:    []string{"script.sh"},
		Timestamp: time.Now().UTC(),
	}
	if err := TrackInstall(workDir, bundle); err != nil {
		t.Fatalf("TrackInstall error = %v", err)
	}

	// Verify installed.json exists with correct content.
	dest := filepath.Join(mushDir, installedFileName)
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("installed.json missing after TrackInstall: %v", err)
	}

	// Verify no temp files left behind.
	entries, err := os.ReadDir(mushDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}

	for _, e := range entries {
		if e.Name() != installedFileName {
			t.Fatalf("unexpected file in .mush/: %s", e.Name())
		}
	}

	// Verify content is valid by round-tripping.
	installed, err := LoadInstalled(workDir)
	if err != nil {
		t.Fatalf("LoadInstalled() error = %v", err)
	}

	if len(installed) != 1 || installed[0].Slug != "test" {
		t.Fatalf("LoadInstalled() = %+v, want [{slug:test}]", installed)
	}
}
