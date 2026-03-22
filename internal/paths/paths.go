package paths

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const appName = "musher"

// rootWithFallback resolves a directory root using a 4-tier priority:
//  1. Branded env var (e.g., MUSHER_CONFIG_HOME) — must be absolute, silently ignored otherwise
//  2. MUSHER_HOME umbrella — must be absolute, silently ignored otherwise
//  3. XDG env var (e.g., XDG_CONFIG_HOME) — must be absolute, silently ignored otherwise
//  4. OS-specific default function → home-dir fallback
func rootWithFallback(brandedEnv, homeSubdir, xdgEnv string, osFn func() (string, error), fallbackDir string) (string, error) {
	// Priority 1: Branded env var (e.g., MUSHER_CONFIG_HOME).
	if v := os.Getenv(brandedEnv); v != "" && filepath.IsAbs(v) {
		return v, nil
	}

	// Priority 2: MUSHER_HOME umbrella.
	if home := os.Getenv("MUSHER_HOME"); home != "" && filepath.IsAbs(home) {
		return filepath.Join(home, homeSubdir), nil
	}

	// Priority 3: XDG env var.
	if xdgEnv != "" {
		if xdg := os.Getenv(xdgEnv); xdg != "" && filepath.IsAbs(xdg) {
			return filepath.Join(xdg, appName), nil
		}
	}

	// Priority 4: OS-specific default, then home-dir fallback.
	if osFn != nil {
		root, err := osFn()
		if err == nil && root != "" {
			return filepath.Join(root, appName), nil
		}
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil && home != "" {
		return filepath.Join(home, fallbackDir, appName), nil
	}

	return "", fmt.Errorf("resolve user home directory")
}

func configRoot() (string, error) {
	return rootWithFallback("MUSHER_CONFIG_HOME", "config", "XDG_CONFIG_HOME", os.UserConfigDir, ".config")
}

func dataRoot() (string, error) {
	noOSDefault := func() (string, error) {
		return "", fmt.Errorf("no OS data directory function")
	}

	return rootWithFallback("MUSHER_DATA_HOME", "data", "XDG_DATA_HOME", noOSDefault, filepath.Join(".local", "share"))
}

func stateRoot() (string, error) {
	noOSDefault := func() (string, error) {
		return "", fmt.Errorf("no OS state directory function")
	}

	return rootWithFallback("MUSHER_STATE_HOME", "state", "XDG_STATE_HOME", noOSDefault, filepath.Join(".local", "state"))
}

func cacheRoot() (string, error) {
	return rootWithFallback("MUSHER_CACHE_HOME", "cache", "XDG_CACHE_HOME", os.UserCacheDir, ".cache")
}

func runtimeRoot() (string, error) {
	// Priority 1: Branded env var.
	if v := os.Getenv("MUSHER_RUNTIME_DIR"); v != "" && filepath.IsAbs(v) {
		return v, nil
	}

	// Priority 2: MUSHER_HOME umbrella.
	if home := os.Getenv("MUSHER_HOME"); home != "" && filepath.IsAbs(home) {
		return filepath.Join(home, "runtime"), nil
	}

	// Priority 3: XDG_RUNTIME_DIR (Linux).
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, appName), nil
	}

	// Priority 4: Temp-based fallback.
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), appName, "run"), nil
	}

	return filepath.Join(os.TempDir(), appName, "run"), nil
}

// ConfigRoot returns the user config root directory.
func ConfigRoot() (string, error) {
	return configRoot()
}

// DataRoot returns the user data root directory.
func DataRoot() (string, error) {
	return dataRoot()
}

// StateRoot returns the user state root directory.
func StateRoot() (string, error) {
	return stateRoot()
}

// CacheRoot returns the user cache root directory.
func CacheRoot() (string, error) {
	return cacheRoot()
}

// RuntimeRoot returns the runtime root directory.
func RuntimeRoot() (string, error) {
	return runtimeRoot()
}

// LogsDir returns the default log directory.
func LogsDir() (string, error) {
	root, err := stateRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "logs"), nil
}

// DefaultLogFile returns the default log file path.
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

// CredentialFilePath returns the host-scoped credential fallback file path.
// The hostID should come from HostIDFromURL.
func CredentialFilePath(hostID string) (string, error) {
	root, err := dataRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "credentials", hostID, "api-key"), nil
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

// HostIDFromURL returns a filesystem-safe host identifier from an API URL.
// Default ports (443 for HTTPS, 80 for HTTP) are omitted.
// Non-default ports are appended with an underscore separator.
func HostIDFromURL(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Host == "" {
		// Best-effort: return cleaned URL as-is.
		return sanitizeHostID(apiURL)
	}

	hostname := parsed.Hostname()
	port := parsed.Port()

	// Omit default ports.
	if port == "" || (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		return sanitizeHostID(hostname)
	}

	return sanitizeHostID(hostname + "_" + port)
}

// KeyringServiceFromURL returns the keyring service name for a given API URL.
// Format: "musher/{hostname}" or "musher/{hostname}:{port}" for non-default ports.
func KeyringServiceFromURL(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Host == "" {
		return "musher/" + sanitizeHostID(apiURL)
	}

	hostname := parsed.Hostname()
	port := parsed.Port()

	// Omit default ports.
	if port == "" || (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		return "musher/" + hostname
	}

	return "musher/" + hostname + ":" + port
}

// sanitizeHostID replaces characters that are problematic in filesystem paths.
func sanitizeHostID(s string) string {
	return strings.NewReplacer(
		"/", "_",
		":", "_",
		"\\", "_",
	).Replace(s)
}
