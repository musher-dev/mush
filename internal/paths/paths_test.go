package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()

	for _, env := range []string{
		"MUSHER_HOME",
		"MUSHER_CONFIG_HOME", "MUSHER_DATA_HOME", "MUSHER_STATE_HOME",
		"MUSHER_CACHE_HOME", "MUSHER_RUNTIME_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
		"XDG_CACHE_HOME", "XDG_RUNTIME_DIR",
	} {
		t.Setenv(env, "")
	}
}

func TestConfigRoot_UsesXDGConfigHome(t *testing.T) {
	clearEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "musher")
	if got != want {
		t.Fatalf("ConfigRoot() = %q, want %q", got, want)
	}
}

func TestCacheRoot_UsesXDGCacheHome(t *testing.T) {
	clearEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	got, err := CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "musher")
	if got != want {
		t.Fatalf("CacheRoot() = %q, want %q", got, want)
	}
}

func TestStateRoot_UsesXDGStateHome(t *testing.T) {
	clearEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)

	got, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "musher")
	if got != want {
		t.Fatalf("StateRoot() = %q, want %q", got, want)
	}
}

func TestStateRoot_FallsBackToLocalState(t *testing.T) {
	clearEnv(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	got, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	want := filepath.Join(home, ".local", "state", "musher")
	if got != want {
		t.Fatalf("StateRoot() = %q, want %q", got, want)
	}
}

func TestDataRoot_UsesXDGDataHome(t *testing.T) {
	clearEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	got, err := DataRoot()
	if err != nil {
		t.Fatalf("DataRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "musher")
	if got != want {
		t.Fatalf("DataRoot() = %q, want %q", got, want)
	}
}

func TestDataRoot_FallsBackToLocalShare(t *testing.T) {
	clearEnv(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	got, err := DataRoot()
	if err != nil {
		t.Fatalf("DataRoot() error = %v", err)
	}

	want := filepath.Join(home, ".local", "share", "musher")
	if got != want {
		t.Fatalf("DataRoot() = %q, want %q", got, want)
	}
}

func TestRuntimeRoot_UsesXDGRuntimeDir(t *testing.T) {
	clearEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	got, err := RuntimeRoot()
	if err != nil {
		t.Fatalf("RuntimeRoot() error = %v", err)
	}

	want := filepath.Join(tmp, "musher")
	if got != want {
		t.Fatalf("RuntimeRoot() = %q, want %q", got, want)
	}
}

func TestRuntimeRoot_FallsBackToTemp(t *testing.T) {
	clearEnv(t)

	got, err := RuntimeRoot()
	if err != nil {
		t.Fatalf("RuntimeRoot() error = %v", err)
	}

	wantSuffix := filepath.Join("musher", "run")
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("RuntimeRoot() = %q, want suffix %q", got, wantSuffix)
	}
}

func TestDerivedPaths(t *testing.T) {
	clearEnv(t)

	cfg := t.TempDir()
	data := t.TempDir()
	state := t.TempDir()
	cache := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_CACHE_HOME", cache)

	logFile, err := DefaultLogFile()
	if err != nil {
		t.Fatalf("DefaultLogFile() error = %v", err)
	}

	wantLog := filepath.Join(state, "musher", "logs", "mush.log")
	if logFile != wantLog {
		t.Fatalf("DefaultLogFile() = %q, want %q", logFile, wantLog)
	}

	stateFile, err := UpdateStateFile()
	if err != nil {
		t.Fatalf("UpdateStateFile() error = %v", err)
	}

	wantState := filepath.Join(state, "musher", "update-check.json")
	if stateFile != wantState {
		t.Fatalf("UpdateStateFile() = %q, want %q", stateFile, wantState)
	}

	credFile, err := CredentialFilePath("api.musher.dev")
	if err != nil {
		t.Fatalf("CredentialFilePath() error = %v", err)
	}

	wantCreds := filepath.Join(data, "musher", "credentials", "api.musher.dev", "api-key")
	if credFile != wantCreds {
		t.Fatalf("CredentialFilePath() = %q, want %q", credFile, wantCreds)
	}

	historyDir, err := HistoryDir()
	if err != nil {
		t.Fatalf("HistoryDir() error = %v", err)
	}

	wantHistory := filepath.Join(state, "musher", "history")
	if historyDir != wantHistory {
		t.Fatalf("HistoryDir() = %q, want %q", historyDir, wantHistory)
	}

	bundleCacheDir, err := BundleCacheDir()
	if err != nil {
		t.Fatalf("BundleCacheDir() error = %v", err)
	}

	wantBundleCache := filepath.Join(cache, "musher", "bundles")
	if bundleCacheDir != wantBundleCache {
		t.Fatalf("BundleCacheDir() = %q, want %q", bundleCacheDir, wantBundleCache)
	}
}

func TestXDGRelativePathIgnored(t *testing.T) {
	clearEnv(t)

	relPath := filepath.Join("relative", "path")

	t.Setenv("XDG_CONFIG_HOME", relPath)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if got == filepath.Join(relPath, "musher") {
		t.Fatal("ConfigRoot() should ignore relative XDG_CONFIG_HOME, but used it")
	}

	t.Setenv("XDG_STATE_HOME", relPath)

	got, err = StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	if got == filepath.Join(relPath, "musher") {
		t.Fatal("StateRoot() should ignore relative XDG_STATE_HOME, but used it")
	}
}

func TestXDGOverridesOSDefault(t *testing.T) {
	clearEnv(t)

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

	if configRoot != filepath.Join(xdgConfig, "musher") {
		t.Fatalf("ConfigRoot() = %q, want XDG override %q", configRoot, filepath.Join(xdgConfig, "musher"))
	}

	cacheRoot, err := CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot() error = %v", err)
	}

	if cacheRoot != filepath.Join(xdgCache, "musher") {
		t.Fatalf("CacheRoot() = %q, want XDG override %q", cacheRoot, filepath.Join(xdgCache, "musher"))
	}

	stateRoot, err := StateRoot()
	if err != nil {
		t.Fatalf("StateRoot() error = %v", err)
	}

	if stateRoot != filepath.Join(xdgState, "musher") {
		t.Fatalf("StateRoot() = %q, want XDG override %q", stateRoot, filepath.Join(xdgState, "musher"))
	}
}

func TestBrandedEnvOverridesXDG(t *testing.T) {
	clearEnv(t)

	branded := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("MUSHER_CONFIG_HOME", branded)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	// Branded env returns the path directly (not appending appName).
	if got != branded {
		t.Fatalf("ConfigRoot() = %q, want branded override %q", got, branded)
	}
}

func TestMusherHomeUmbrella(t *testing.T) {
	clearEnv(t)

	home := t.TempDir()
	t.Setenv("MUSHER_HOME", home)

	tests := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"ConfigRoot", ConfigRoot, filepath.Join(home, "config")},
		{"DataRoot", DataRoot, filepath.Join(home, "data")},
		{"StateRoot", StateRoot, filepath.Join(home, "state")},
		{"CacheRoot", CacheRoot, filepath.Join(home, "cache")},
		{"RuntimeRoot", RuntimeRoot, filepath.Join(home, "runtime")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}

			if got != tt.want {
				t.Fatalf("%s() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBrandedEnvRelativeIgnored(t *testing.T) {
	clearEnv(t)

	t.Setenv("MUSHER_CONFIG_HOME", "relative/path")

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if got == "relative/path" {
		t.Fatal("ConfigRoot() should ignore relative MUSHER_CONFIG_HOME, but used it")
	}
}

func TestMusherHomeRelativeIgnored(t *testing.T) {
	clearEnv(t)

	t.Setenv("MUSHER_HOME", filepath.Join("relative", "home"))

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if got == filepath.Join("relative", "home", "config") {
		t.Fatal("ConfigRoot() should ignore relative MUSHER_HOME, but used it")
	}
}

func TestBrandedEnvOverridesMusherHome(t *testing.T) {
	clearEnv(t)

	branded := t.TempDir()
	home := t.TempDir()
	t.Setenv("MUSHER_CONFIG_HOME", branded)
	t.Setenv("MUSHER_HOME", home)

	got, err := ConfigRoot()
	if err != nil {
		t.Fatalf("ConfigRoot() error = %v", err)
	}

	if got != branded {
		t.Fatalf("ConfigRoot() = %q, want branded %q (should override MUSHER_HOME)", got, branded)
	}
}

func TestHostIDFromURL(t *testing.T) {
	tests := []struct {
		apiURL string
		want   string
	}{
		{"https://api.musher.dev", "api.musher.dev"},
		{"https://api.musher.dev:443", "api.musher.dev"},
		{"http://api.musher.dev:80", "api.musher.dev"},
		{"https://api.musher.dev:8443", "api.musher.dev_8443"},
		{"http://localhost:17201", "localhost_17201"},
		{"http://host.docker.internal:17201", "host.docker.internal_17201"},
	}

	for _, tt := range tests {
		t.Run(tt.apiURL, func(t *testing.T) {
			got := HostIDFromURL(tt.apiURL)
			if got != tt.want {
				t.Fatalf("HostIDFromURL(%q) = %q, want %q", tt.apiURL, got, tt.want)
			}
		})
	}
}

func TestKeyringServiceFromURL(t *testing.T) {
	tests := []struct {
		apiURL string
		want   string
	}{
		{"https://api.musher.dev", "musher/api.musher.dev"},
		{"https://api.musher.dev:443", "musher/api.musher.dev"},
		{"http://api.musher.dev:80", "musher/api.musher.dev"},
		{"https://api.musher.dev:8443", "musher/api.musher.dev:8443"},
		{"http://localhost:17201", "musher/localhost:17201"},
	}

	for _, tt := range tests {
		t.Run(tt.apiURL, func(t *testing.T) {
			got := KeyringServiceFromURL(tt.apiURL)
			if got != tt.want {
				t.Fatalf("KeyringServiceFromURL(%q) = %q, want %q", tt.apiURL, got, tt.want)
			}
		})
	}
}

func TestCredentialFilePath_HostScoped(t *testing.T) {
	clearEnv(t)

	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)

	got, err := CredentialFilePath("api.musher.dev")
	if err != nil {
		t.Fatalf("CredentialFilePath() error = %v", err)
	}

	want := filepath.Join(data, "musher", "credentials", "api.musher.dev", "api-key")
	if got != want {
		t.Fatalf("CredentialFilePath() = %q, want %q", got, want)
	}

	got2, err := CredentialFilePath("localhost_17201")
	if err != nil {
		t.Fatalf("CredentialFilePath() error = %v", err)
	}

	want2 := filepath.Join(data, "musher", "credentials", "localhost_17201", "api-key")
	if got2 != want2 {
		t.Fatalf("CredentialFilePath() = %q, want %q", got2, want2)
	}

	// Different hosts produce different paths.
	if got == got2 {
		t.Fatal("CredentialFilePath() should produce different paths for different hosts")
	}
}
