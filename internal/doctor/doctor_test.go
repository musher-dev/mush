package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func clearDoctorEnv(t *testing.T) {
	t.Helper()

	for _, env := range []string{
		"MUSHER_HOME", "MUSHER_CONFIG_HOME", "MUSHER_DATA_HOME",
		"MUSHER_STATE_HOME", "MUSHER_CACHE_HOME", "MUSHER_RUNTIME_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	} {
		t.Setenv(env, "")
	}
}

func TestCheckDirectoryStructure_AllExist(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Create the directories (paths package appends "musher")
	for _, name := range []string{"config/musher", "state/musher", "cache/musher"} {
		if err := os.MkdirAll(filepath.Join(tmp, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	result := checkDirectoryStructure(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckDirectoryStructure_Missing(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Don't create directories — they should be missing
	result := checkDirectoryStructure(t.Context())
	if result.Status != StatusWarn {
		t.Errorf("expected WARN, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckDirectoryStructure_NotADir(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Create config/musher as a file instead of directory
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "musher"), []byte("oops"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkDirectoryStructure(t.Context())
	if result.Status != StatusFail {
		t.Errorf("expected FAIL, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckConfigFile_NoFile(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	result := checkConfigFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s", result.Status, result.Message)
	}

	if result.Message != "No config file (using defaults)" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestCheckConfigFile_ValidYAML(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "musher")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("api:\n  url: https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkConfigFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckConfigFile_InvalidYAML(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "musher")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":\n  bad: [yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkConfigFile(t.Context())
	if result.Status != StatusFail {
		t.Errorf("expected FAIL, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckCredentialsFile_NoFile(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s", result.Status, result.Message)
	}

	if result.Message != "Not present (using keyring or env)" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestCheckCredentialsFile_Secure(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Credentials are now host-scoped under data root
	credDir := filepath.Join(tmp, "musher", "credentials", "api.musher.dev")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}

	credFile := filepath.Join(credDir, "api-key")
	if err := os.WriteFile(credFile, []byte("sa_test_key"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckCredentialsFile_TooPermissive(t *testing.T) {
	clearDoctorEnv(t)

	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Credentials are now host-scoped under data root
	credDir := filepath.Join(tmp, "musher", "credentials", "api.musher.dev")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}

	credFile := filepath.Join(credDir, "api-key")
	if err := os.WriteFile(credFile, []byte("sa_test_key"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusWarn {
		t.Errorf("expected WARN, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}
