package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckDirectoryStructure_AllExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Create the directories (paths package appends "mush")
	for _, name := range []string{"config/mush", "state/mush", "cache/mush"} {
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
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	// Create config/mush as a file instead of directory
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "mush"), []byte("oops"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkDirectoryStructure(t.Context())
	if result.Status != StatusFail {
		t.Errorf("expected FAIL, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckConfigFile_NoFile(t *testing.T) {
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
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "mush")
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
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "mush")
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
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s", result.Status, result.Message)
	}

	if result.Message != "Not present (using keyring or env)" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestCheckCredentialsFile_Secure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "mush")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	credFile := filepath.Join(configDir, "api-key")
	if err := os.WriteFile(credFile, []byte("sa_test_key"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}

func TestCheckCredentialsFile_TooPermissive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	configDir := filepath.Join(tmp, "mush")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	credFile := filepath.Join(configDir, "api-key")
	if err := os.WriteFile(credFile, []byte("sa_test_key"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := checkCredentialsFile(t.Context())
	if result.Status != StatusWarn {
		t.Errorf("expected WARN, got %v: %s — %s", result.Status, result.Message, result.Detail)
	}
}
