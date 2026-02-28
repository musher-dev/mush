package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "mush"

func configRoot() (string, error) {
	return rootWithFallback("XDG_CONFIG_HOME", os.UserConfigDir, ".config")
}

func stateRoot() (string, error) {
	noOSDefault := func() (string, error) {
		return "", fmt.Errorf("no OS state directory function")
	}

	return rootWithFallback("XDG_STATE_HOME", noOSDefault, filepath.Join(".local", "state"))
}

func cacheRoot() (string, error) {
	return rootWithFallback("XDG_CACHE_HOME", os.UserCacheDir, ".cache")
}

func rootWithFallback(xdgEnv string, osFn func() (string, error), fallbackDir string) (string, error) {
	// Priority 1: Explicit XDG env var (cross-platform).
	if xdg := os.Getenv(xdgEnv); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, appName), nil
	}

	// Priority 2: OS-specific default (macOS ~/Library/..., Windows %AppData%, Linux ~/.config).
	root, err := osFn()
	if err == nil && root != "" {
		return filepath.Join(root, appName), nil
	}

	// Priority 3: Home-dir fallback.
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

// StateRoot returns the user state root directory for Mush.
func StateRoot() (string, error) {
	return stateRoot()
}

// CacheRoot returns the user cache root directory for Mush.
func CacheRoot() (string, error) {
	return cacheRoot()
}

// LogsDir returns the default log directory for Mush.
func LogsDir() (string, error) {
	root, err := stateRoot()
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
	root, err := stateRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "update-check.json"), nil
}

// CredentialsFile returns the credential fallback file path.
func CredentialsFile() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "api-key"), nil
}

// HistoryDir returns the default transcript history directory.
func HistoryDir() (string, error) {
	root, err := stateRoot()
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

	return filepath.Join(root, "bundles"), nil
}
