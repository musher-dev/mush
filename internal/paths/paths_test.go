package paths

import (
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

func TestDerivedPaths(t *testing.T) {
	cfg := t.TempDir()
	cache := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", cache)

	logFile, err := DefaultLogFile()
	if err != nil {
		t.Fatalf("DefaultLogFile() error = %v", err)
	}

	wantLog := filepath.Join(cfg, "mush", "logs", "mush.log")
	if logFile != wantLog {
		t.Fatalf("DefaultLogFile() = %q, want %q", logFile, wantLog)
	}

	stateFile, err := UpdateStateFile()
	if err != nil {
		t.Fatalf("UpdateStateFile() error = %v", err)
	}

	wantState := filepath.Join(cfg, "mush", "update-check")
	if stateFile != wantState {
		t.Fatalf("UpdateStateFile() = %q, want %q", stateFile, wantState)
	}

	credFile, err := CredentialsFile()
	if err != nil {
		t.Fatalf("CredentialsFile() error = %v", err)
	}

	wantCreds := filepath.Join(cfg, "mush", "credentials")
	if credFile != wantCreds {
		t.Fatalf("CredentialsFile() = %q, want %q", credFile, wantCreds)
	}

	historyDir, err := HistoryDir()
	if err != nil {
		t.Fatalf("HistoryDir() error = %v", err)
	}

	wantHistory := filepath.Join(cfg, "mush", "history")
	if historyDir != wantHistory {
		t.Fatalf("HistoryDir() = %q, want %q", historyDir, wantHistory)
	}

	bundleCacheDir, err := BundleCacheDir()
	if err != nil {
		t.Fatalf("BundleCacheDir() error = %v", err)
	}

	wantBundleCache := filepath.Join(cache, "mush", "cache")
	if bundleCacheDir != wantBundleCache {
		t.Fatalf("BundleCacheDir() = %q, want %q", bundleCacheDir, wantBundleCache)
	}
}
