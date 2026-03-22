// Package auth handles credential storage and retrieval for Mush.
//
// Credentials are sourced in the following priority order:
//  1. Environment variable: MUSHER_API_KEY
//  2. OS Keyring (service name derived from API URL: musher/{host})
//  3. Data file fallback: <data root>/credentials/<hostID>/api-key
package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/paths"
	"github.com/musher-dev/mush/internal/safeio"
	"github.com/zalando/go-keyring"
)

const (
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
	SourceFile    CredentialSource = "credentials file"
	SourceNone    CredentialSource = ""
)

// GetCredentials returns the API key and its source for the given API URL.
// Returns empty strings if no credentials are found.
func GetCredentials(apiURL string) (source CredentialSource, apiKey string) {
	// Priority 1: Environment variable
	if key := os.Getenv(envVarName); key != "" {
		return SourceEnv, key
	}

	// Priority 2: OS Keyring (host-scoped)
	service := paths.KeyringServiceFromURL(apiURL)
	if key, err := keyring.Get(service, keyringUser); err == nil && key != "" {
		return SourceKeyring, key
	}

	// Priority 3: Data file fallback (host-scoped)
	if key := readCredentialsFile(apiURL); key != "" {
		return SourceFile, key
	}

	return SourceNone, ""
}

// StoreAPIKey stores the API key for the given API URL in the OS keyring.
// Falls back to file storage if keyring is unavailable.
func StoreAPIKey(apiURL, apiKey string) error {
	// Try keyring first
	service := paths.KeyringServiceFromURL(apiURL)

	err := keyring.Set(service, keyringUser, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return writeCredentialsFile(apiURL, apiKey)
}

// DeleteAPIKey removes the stored API key for the given API URL.
func DeleteAPIKey(apiURL string) error {
	service := paths.KeyringServiceFromURL(apiURL)

	// Try to delete from keyring
	keyringErr := keyring.Delete(service, keyringUser)

	// Also try to delete from file
	fileErr := deleteCredentialsFile(apiURL)

	// Return error only if both failed and nothing was deleted
	if keyringErr != nil && fileErr != nil {
		return fmt.Errorf("no stored credentials found")
	}

	return nil
}

// credentialFilePath returns the host-scoped credential file path for the given API URL.
func credentialFilePath(apiURL string) string {
	hostID := paths.HostIDFromURL(apiURL)

	path, err := paths.CredentialFilePath(hostID)
	if err != nil {
		return ""
	}

	return filepath.Clean(path)
}

// readCredentialsFile reads the API key from the host-scoped file fallback.
func readCredentialsFile(apiURL string) string {
	path := credentialFilePath(apiURL)
	if path == "" {
		return ""
	}

	data, err := safeio.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// writeCredentialsFile writes the API key to the host-scoped file fallback.
func writeCredentialsFile(apiURL, apiKey string) error {
	path := credentialFilePath(apiURL)
	if path == "" {
		return fmt.Errorf("could not determine data directory")
	}

	// Create directory with secure permissions
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Write file with secure permissions (owner read/write only)
	if err := os.WriteFile(path, []byte(apiKey+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// deleteCredentialsFile removes the host-scoped credentials file.
func deleteCredentialsFile(apiURL string) error {
	path := credentialFilePath(apiURL)
	if path == "" {
		return fmt.Errorf("could not determine data directory")
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
