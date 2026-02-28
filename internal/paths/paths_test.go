package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoot_UsesXDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "mush")
	if got != want {
		t.Fatalf("ConfigRoot() = %q, want %q", got, want)
	}
}

func TestCacheRoot_UsesXDGCacheHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	got, err := CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "mush")
	if got != want {
		t.Fatalf("CacheRoot() = %q, want %q", got, want)
	}
}

func TestStateRoot_UsesXDGStateHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	got, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "mush")
	if got != want {
		t.Fatalf("StateRoot() = %q, want %q", got, want)
	}
}

func TestStateRoot_FallsBackToLocalState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	got, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	want := filepath.Join(home, ".local", "state", "mush")
	if got != want {
		t.Fatalf("StateRoot() = %q, want %q", got, want)
	}
}

func TestDerivedPaths(t *testing.T) {
	cfg := t.TempDir()
	state := t.TempDir()
	cache := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_CACHE_HOME", cache)

	logFile, err := DefaultLogFile()
	if err != nil {
		t.Fatalf("DefaultLogFile() error = %v", err)
	}

	wantLog := filepath.Join(state, "mush", "logs", "mush.log")
	if logFile != wantLog {
		t.Fatalf("DefaultLogFile() = %q, want %q", logFile, wantLog)
	}

	stateFile, err := UpdateStateFile()
	if err != nil {
		t.Fatalf("UpdateStateFile() error = %v", err)
	}

	wantState := filepath.Join(state, "mush", "update-check.json")
	if stateFile != wantState {
		t.Fatalf("UpdateStateFile() = %q, want %q", stateFile, wantState)
	}

	credFile, err := CredentialsFile()
	if err != nil {
		t.Fatalf("CredentialsFile() error = %v", err)
	}

	wantCreds := filepath.Join(cfg, "mush", "api-key")
	if credFile != wantCreds {
		t.Fatalf("CredentialsFile() = %q, want %q", credFile, wantCreds)
	}

	historyDir, err := HistoryDir()
	if err != nil {
		t.Fatalf("HistoryDir() error = %v", err)
	}

	wantHistory := filepath.Join(state, "mush", "history")
	if historyDir != wantHistory {
		t.Fatalf("HistoryDir() = %q, want %q", historyDir, wantHistory)
	}

	bundleCacheDir, err := BundleCacheDir()
	if err != nil {
		t.Fatalf("BundleCacheDir() error = %v", err)
	}

	wantBundleCache := filepath.Join(cache, "mush", "bundles")
	if bundleCacheDir != wantBundleCache {
		t.Fatalf("BundleCacheDir() = %q, want %q", bundleCacheDir, wantBundleCache)
	}
}

func TestXDGRelativePathIgnored(t *testing.T) {
	relPath := filepath.Join("relative", "path")

	t.Setenv("XDG_CONFIG_HOME", relPath)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if got == filepath.Join(relPath, "mush") {
		t.Fatal("ConfigRoot() should ignore relative XDG_CONFIG_HOME, but used it")
	}

	t.Setenv("XDG_STATE_HOME", relPath)

	got, err = StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	if got == filepath.Join(relPath, "mush") {
		t.Fatal("StateRoot() should ignore relative XDG_STATE_HOME, but used it")
	}
}

func TestXDGOverridesOSDefault(t *testing.T) {
	xdgConfig := t.TempDir()
	xdgCache := t.TempDir()
	xdgState := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
	t.Setenv("XDG_STATE_HOME", xdgState)

	configRoot, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if configRoot != filepath.Join(xdgConfig, "mush") {
		t.Fatalf("ConfigRoot() = %q, want XDG override %q", configRoot, filepath.Join(xdgConfig, "mush"))
	}

	cacheRoot, err := CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot() error = %v", err)
	}

	if cacheRoot != filepath.Join(xdgCache, "mush") {
		t.Fatalf("CacheRoot() = %q, want XDG override %q", cacheRoot, filepath.Join(xdgCache, "mush"))
	}

	stateRoot, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	if stateRoot != filepath.Join(xdgState, "mush") {
		t.Fatalf("StateRoot() = %q, want XDG override %q", stateRoot, filepath.Join(xdgState, "mush"))
	}
}
