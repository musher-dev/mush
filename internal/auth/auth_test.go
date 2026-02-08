package auth

import (
	"os"
	"path/filepath"
	"testing"
)

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
			// Save original value
			orig := os.Getenv(envVarName)
			defer func() {
				if orig != "" {
					os.Setenv(envVarName, orig)
				} else {
					os.Unsetenv(envVarName)
				}
			}()

			if tt.envKey != "" {
				os.Setenv(envVarName, tt.envKey)
			} else {
				os.Unsetenv(envVarName)
			}

			source, key := GetCredentials()

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

func TestCredentialsFilePath(t *testing.T) {
	path := credentialsFilePath()

	if path == "" {
		t.Skip("Could not determine home directory")
	}

	// Should contain .config/mush/credentials
	if !filepath.IsAbs(path) {
		t.Errorf("credentialsFilePath() = %q, want absolute path", path)
	}

	expectedSuffix := filepath.Join(".config", "mush", "credentials")
	if !containsPath(path, expectedSuffix) {
		t.Errorf("credentialsFilePath() = %q, want to contain %q", path, expectedSuffix)
	}
}

func TestCredentialSource_String(t *testing.T) {
	tests := []struct {
		source CredentialSource
		want   string
	}{
		{SourceEnv, "environment variable"},
		{SourceKeyring, "keyring"},
		{SourceFile, "config file"},
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
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	testKey := "test-api-key-xyz"

	// Write credentials
	err := writeCredentialsFile(testKey)
	if err != nil {
		t.Fatalf("writeCredentialsFile() error = %v", err)
	}

	// Read back
	got := readCredentialsFile()
	if got != testKey {
		t.Errorf("readCredentialsFile() = %q, want %q", got, testKey)
	}

	// Verify file permissions
	path := credentialsFilePath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}

	// Check permissions (0600 = owner read/write only)
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("credentials file permissions = %o, want 0600", perm)
	}
}

func TestDeleteCredentialsFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Write credentials first
	err := writeCredentialsFile("test-key")
	if err != nil {
		t.Fatalf("writeCredentialsFile() error = %v", err)
	}

	// Delete
	err = deleteCredentialsFile()
	if err != nil {
		t.Errorf("deleteCredentialsFile() error = %v", err)
	}

	// Verify file is gone
	path := credentialsFilePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("credentials file still exists after delete")
	}
}

func TestDeleteCredentialsFile_NotFound(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Try to delete non-existent file
	err := deleteCredentialsFile()
	if err == nil {
		t.Errorf("deleteCredentialsFile() should return error for non-existent file")
	}
}

// containsPath checks if path contains the expectedSuffix.
func containsPath(path, expectedSuffix string) bool {
	return len(path) >= len(expectedSuffix) &&
		path[len(path)-len(expectedSuffix):] == expectedSuffix
}
