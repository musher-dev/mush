package auth

import (
	"os"
	"path/filepath"
	"testing"
)

const testAPIURL = "https://api.musher.dev"

func clearAuthEnv(t *testing.T) {
	t.Helper()

	for _, env := range []string{
		"MUSHER_API_KEY",
		"MUSHER_HOME", "MUSHER_DATA_HOME",
		"XDG_DATA_HOME", "XDG_CONFIG_HOME",
	} {
		t.Setenv(env, "")
	}
}

func TestGetCredentials_FromEnv(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		wantSource CredentialSource
		wantKey    string
	}{
		{
			name:       "from environment variable",
			envKey:     "test-api-key-123",
			wantSource: SourceEnv,
			wantKey:    "test-api-key-123",
		},
		{
			name:       "empty environment variable",
			envKey:     "",
			wantSource: SourceNone,
			wantKey:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAuthEnv(t)

			if tt.envKey != "" {
				t.Setenv(envVarName, tt.envKey)
			} else {
				t.Setenv(envVarName, "")
				os.Unsetenv(envVarName)
			}

			source, key := GetCredentials(testAPIURL)

			// Environment variable has highest priority
			if tt.envKey != "" {
				if source != tt.wantSource {
					t.Errorf("source = %v, want %v", source, tt.wantSource)
				}

				if key != tt.wantKey {
					t.Errorf("key = %v, want %v", key, tt.wantKey)
				}
			}
		})
	}
}

func TestCredentialFilePath_HostScoped(t *testing.T) {
	clearAuthEnv(t)

	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	path := credentialFilePath(testAPIURL)

	if path == "" {
		t.Skip("Could not determine data directory")
	}

	if !filepath.IsAbs(path) {
		t.Errorf("credentialFilePath() = %q, want absolute path", path)
	}

	expectedSuffix := filepath.Join("musher", "credentials", "api.musher.dev", "api-key")
	if !containsPath(path, expectedSuffix) {
		t.Errorf("credentialFilePath() = %q, want to contain %q", path, expectedSuffix)
	}
}

func TestCredentialFilePath_DifferentHosts(t *testing.T) {
	clearAuthEnv(t)

	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	path1 := credentialFilePath("https://api.musher.dev")
	path2 := credentialFilePath("http://localhost:17201")

	if path1 == path2 {
		t.Errorf("different API URLs should produce different credential paths, both got %q", path1)
	}
}

func TestCredentialSource_String(t *testing.T) {
	tests := []struct {
		source CredentialSource
		want   string
	}{
		{SourceEnv, "environment variable"},
		{SourceKeyring, "keyring"},
		{SourceFile, "credentials file"},
		{SourceNone, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			if got := string(tt.source); got != tt.want {
				t.Errorf("CredentialSource = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteAndReadCredentialsFile(t *testing.T) {
	clearAuthEnv(t)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))

	testKey := "test-api-key-xyz"

	err := writeCredentialsFile(testAPIURL, testKey)
	if err != nil {
		t.Fatalf("writeCredentialsFile() error = %v", err)
	}

	got := readCredentialsFile(testAPIURL)
	if got != testKey {
		t.Errorf("readCredentialsFile() = %q, want %q", got, testKey)
	}

	// Verify file permissions
	path := credentialFilePath(testAPIURL)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("credentials file permissions = %o, want 0600", perm)
	}
}

func TestDeleteCredentialsFile(t *testing.T) {
	clearAuthEnv(t)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))

	err := writeCredentialsFile(testAPIURL, "test-key")
	if err != nil {
		t.Fatalf("writeCredentialsFile() error = %v", err)
	}

	err = deleteCredentialsFile(testAPIURL)
	if err != nil {
		t.Errorf("deleteCredentialsFile() error = %v", err)
	}

	path := credentialFilePath(testAPIURL)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("credentials file still exists after delete")
	}
}

func TestDeleteCredentialsFile_NotFound(t *testing.T) {
	clearAuthEnv(t)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))

	err := deleteCredentialsFile(testAPIURL)
	if err == nil {
		t.Errorf("deleteCredentialsFile() should return error for non-existent file")
	}
}

// containsPath checks if path contains the expectedSuffix.
func containsPath(path, expectedSuffix string) bool {
	return len(path) >= len(expectedSuffix) &&
		path[len(path)-len(expectedSuffix):] == expectedSuffix
}
