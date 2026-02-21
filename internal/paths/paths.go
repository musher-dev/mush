package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "mush"

func configRoot() (string, error) {
	return rootWithFallback(os.UserConfigDir, ".config")
}

func cacheRoot() (string, error) {
	return rootWithFallback(os.UserCacheDir, ".cache")
}

func rootWithFallback(primary func() (string, error), fallbackDir string) (string, error) {
	root, err := primary()
	if err == nil && root != "" {
		return filepath.Join(root, appName), nil
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil && home != "" {
		return filepath.Join(home, fallbackDir, appName), nil
	}

	if err != nil {
		return "", err
	}

	return "", fmt.Errorf("resolve user home directory")
}

// ConfigRoot returns the user config root directory for Mush.
func ConfigRoot() (string, error) {
	return configRoot()
}

// CacheRoot returns the user cache root directory for Mush.
func CacheRoot() (string, error) {
	return cacheRoot()
}

// LogsDir returns the default log directory for Mush.
func LogsDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "logs"), nil
}

// DefaultLogFile returns the default log file path for Mush.
func DefaultLogFile() (string, error) {
	logsDir, err := LogsDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(logsDir, "mush.log"), nil
}

// UpdateStateFile returns the update state file path.
func UpdateStateFile() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "update-check"), nil
}

// CredentialsFile returns the credential fallback file path.
func CredentialsFile() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "credentials"), nil
}

// HistoryDir returns the default transcript history directory.
func HistoryDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "history"), nil
}

// BundleCacheDir returns the bundle cache directory.
func BundleCacheDir() (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "cache"), nil
}
