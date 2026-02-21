// Package auth handles credential storage and retrieval for Mush.
//
// Credentials are sourced in the following priority order:
//  1. Environment variable: MUSHER_API_KEY
//  2. OS Keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service)
//  3. Config file fallback: <user config dir>/mush/credentials (for non-interactive environments)
package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/paths"
	"github.com/zalando/go-keyring"
)

const (
	// keyringService is the service name used in OS keyring storage.
	keyringService = "mush"
	// keyringUser is the user/account name used in OS keyring storage.
	keyringUser = "api-key"
	// envVarName is the environment variable for the API key.
	envVarName = "MUSHER_API_KEY"
)

// CredentialSource indicates where credentials were found.
type CredentialSource string

// Credential source constants identify where credentials were loaded from.
const (
	SourceEnv     CredentialSource = "environment variable"
	SourceKeyring CredentialSource = "keyring"
	SourceFile    CredentialSource = "config file"
	SourceNone    CredentialSource = ""
)

// GetCredentials returns the API key and its source.
// Returns empty strings if no credentials are found.
func GetCredentials() (source CredentialSource, apiKey string) {
	// Priority 1: Environment variable
	if key := os.Getenv(envVarName); key != "" {
		return SourceEnv, key
	}

	// Priority 2: OS Keyring
	if key, err := keyring.Get(keyringService, keyringUser); err == nil && key != "" {
		return SourceKeyring, key
	}

	// Priority 3: Config file fallback
	if key := readCredentialsFile(); key != "" {
		return SourceFile, key
	}

	return SourceNone, ""
}

// StoreAPIKey stores the API key in the OS keyring.
// Falls back to file storage if keyring is unavailable.
func StoreAPIKey(apiKey string) error {
	// Try keyring first
	err := keyring.Set(keyringService, keyringUser, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return writeCredentialsFile(apiKey)
}

// DeleteAPIKey removes the stored API key.
func DeleteAPIKey() error {
	// Try to delete from keyring
	keyringErr := keyring.Delete(keyringService, keyringUser)

	// Also try to delete from file
	fileErr := deleteCredentialsFile()

	// Return error only if both failed and nothing was deleted
	if keyringErr != nil && fileErr != nil {
		return fmt.Errorf("no stored credentials found")
	}

	return nil
}

// credentialsFilePath returns the path to the credentials file.
func credentialsFilePath() string {
	path, err := paths.CredentialsFile()
	if err != nil {
		return ""
	}

	return filepath.Clean(path)
}

// readCredentialsFile reads the API key from the file fallback.
func readCredentialsFile() string {
	path := credentialsFilePath()
	if path == "" {
		return ""
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path from controlled config directory
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// writeCredentialsFile writes the API key to the file fallback.
func writeCredentialsFile(apiKey string) error {
	path := credentialsFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	// Create directory with secure permissions
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write file with secure permissions (owner read/write only)
	if err := os.WriteFile(path, []byte(apiKey+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// deleteCredentialsFile removes the credentials file.
func deleteCredentialsFile() error {
	path := credentialsFilePath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}

	err := os.Remove(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("credentials file not found")
	}

	if err != nil {
		return fmt.Errorf("remove credentials file: %w", err)
	}

	return nil
}
