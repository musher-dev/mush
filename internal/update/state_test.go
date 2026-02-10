package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// setTestHome overrides all home-related env vars for cross-platform test isolation.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

func TestLoadState_NoFile(t *testing.T) {
	tmp := t.TempDir()
	setTestHome(t, tmp)

	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if !state.LastCheckedAt.IsZero() {
		t.Errorf("expected zero LastCheckedAt, got %v", state.LastCheckedAt)
	}
	if state.LatestVersion != "" {
		t.Errorf("expected empty LatestVersion, got %q", state.LatestVersion)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	tmp := t.TempDir()
	setTestHome(t, tmp)

	now := time.Now().Truncate(time.Second)
	original := &State{
		LastCheckedAt:  now,
		LatestVersion:  "1.2.3",
		CurrentVersion: "1.0.0",
		ReleaseURL:     "https://example.com/release",
	}

	if err := SaveState(original); err != nil {
		t.Fatalf("SaveState returned error: %v", err)
	}

	// Verify the file exists
	stateFile := filepath.Join(tmp, ".config", "mush", stateFileName)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("state file was not created")
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}

	if !loaded.LastCheckedAt.Equal(now) {
		t.Errorf("LastCheckedAt: got %v, want %v", loaded.LastCheckedAt, now)
	}
	if loaded.LatestVersion != "1.2.3" {
		t.Errorf("LatestVersion: got %q, want %q", loaded.LatestVersion, "1.2.3")
	}
	if loaded.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion: got %q, want %q", loaded.CurrentVersion, "1.0.0")
	}
	if loaded.ReleaseURL != "https://example.com/release" {
		t.Errorf("ReleaseURL: got %q, want %q", loaded.ReleaseURL, "https://example.com/release")
	}
}

func TestSaveState_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	setTestHome(t, tmp)

	first := &State{LatestVersion: "1.0.0", LastCheckedAt: time.Now()}
	if err := SaveState(first); err != nil {
		t.Fatalf("first SaveState: %v", err)
	}

	second := &State{LatestVersion: "2.0.0", LastCheckedAt: time.Now()}
	if err := SaveState(second); err != nil {
		t.Fatalf("second SaveState: %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.LatestVersion != "2.0.0" {
		t.Errorf("expected 2.0.0 after overwrite, got %q", loaded.LatestVersion)
	}
}

func TestShouldCheck_Fresh(t *testing.T) {
	state := &State{
		LastCheckedAt: time.Now(),
	}
	if state.ShouldCheck() {
		t.Error("ShouldCheck returned true for fresh state")
	}
}

func TestShouldCheck_Stale(t *testing.T) {
	state := &State{
		LastCheckedAt: time.Now().Add(-25 * time.Hour),
	}
	if !state.ShouldCheck() {
		t.Error("ShouldCheck returned false for stale state")
	}
}

func TestShouldCheck_ZeroTime(t *testing.T) {
	state := &State{}
	if !state.ShouldCheck() {
		t.Error("ShouldCheck returned false for zero-time state")
	}
}

func TestHasUpdate(t *testing.T) {
	tests := []struct {
		name    string
		state   State
		current string
		want    bool
	}{
		{
			name:    "newer available",
			state:   State{LatestVersion: "2.0.0"},
			current: "1.0.0",
			want:    true,
		},
		{
			name:    "same version",
			state:   State{LatestVersion: "1.0.0"},
			current: "1.0.0",
			want:    false,
		},
		{
			name:    "older cached",
			state:   State{LatestVersion: "0.9.0"},
			current: "1.0.0",
			want:    false,
		},
		{
			name:    "empty latest",
			state:   State{LatestVersion: ""},
			current: "1.0.0",
			want:    false,
		},
		{
			name:    "empty current",
			state:   State{LatestVersion: "2.0.0"},
			current: "",
			want:    false,
		},
		{
			name:    "dev current",
			state:   State{LatestVersion: "2.0.0"},
			current: "dev",
			want:    false,
		},
		{
			name:    "invalid latest",
			state:   State{LatestVersion: "not-a-version"},
			current: "1.0.0",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.HasUpdate(tt.current)
			if got != tt.want {
				t.Errorf("HasUpdate(%q) = %v, want %v", tt.current, got, tt.want)
			}
		})
	}
}

func TestLoadState_CorruptedFile(t *testing.T) {
	tmp := t.TempDir()
	setTestHome(t, tmp)

	dir := filepath.Join(tmp, ".config", "mush")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write garbage data
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("not json{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error for corrupted file: %v", err)
	}
	if !state.LastCheckedAt.IsZero() {
		t.Error("expected zero-value state for corrupted file")
	}
}
