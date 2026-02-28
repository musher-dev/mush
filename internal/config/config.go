// Package config handles Mush configuration using Viper.
//
// Configuration sources (in priority order):
//  1. Environment variables (MUSH_*)
//  2. Config file (<user config dir>/mush/config.yaml)
//  3. Built-in defaults
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/musher-dev/mush/internal/paths"
)

const (
	// DefaultAPIURL is the default Musher API endpoint.
	DefaultAPIURL = "https://api.musher.dev"
	// DefaultPollInterval is the default poll interval as a duration string.
	DefaultPollInterval = "30s"
	// DefaultHeartbeatInterval is the default heartbeat interval as a duration string.
	DefaultHeartbeatInterval = "30s"
)

const (
	defaultPollIntervalDuration      = 30 * time.Second
	defaultHeartbeatIntervalDuration = 30 * time.Second
	minIntervalDuration              = 1 * time.Second
)

// Config holds the Mush configuration.
type Config struct {
	v *viper.Viper
}

// Load reads configuration from all sources.
func Load() *Config {
	v := viper.New()

	// Set defaults
	v.SetDefault("api.url", DefaultAPIURL)
	v.SetDefault("worker.poll_interval", DefaultPollInterval)
	v.SetDefault("worker.heartbeat_interval", DefaultHeartbeatInterval)
	v.SetDefault("history.enabled", true)
	v.SetDefault("history.scrollback_lines", 10000)
	v.SetDefault("history.retention", (30 * 24 * time.Hour).String())

	// Config file location
	configDir, err := paths.ConfigRoot()
	if err == nil {
		historyDir, historyErr := paths.HistoryDir()
		if historyErr == nil {
			v.SetDefault("history.dir", historyDir)
		} else {
			if home, homeErr := os.UserHomeDir(); homeErr == nil {
				v.SetDefault("history.dir", filepath.Join(home, ".local", "state", "mush", "history"))
			}
		}

		v.AddConfigPath(configDir)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Environment variables
	v.SetEnvPrefix("MUSH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore if not found, but warn on other errors)
	if err := v.ReadInConfig(); err != nil {
		var configNotFound viper.ConfigFileNotFoundError
		if !errors.As(err, &configNotFound) {
			slog.Default().Warn("error reading config file", "component", "config", "event.type", "config.read.warning", "error", err.Error())
		}
	}

	return &Config{v: v}
}

// Get returns a configuration value.
func (c *Config) Get(key string) interface{} {
	return c.v.Get(key)
}

// GetString returns a configuration value as string.
func (c *Config) GetString(key string) string {
	return c.v.GetString(key)
}

// GetInt returns a configuration value as int.
func (c *Config) GetInt(key string) int {
	return c.v.GetInt(key)
}

// Set sets a configuration value and persists it.
func (c *Config) Set(key string, value interface{}) error {
	c.v.Set(key, value)

	// Ensure config directory exists
	configDir, err := paths.ConfigRoot()
	if err != nil {
		return fmt.Errorf("resolve config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.yaml")

	if err := c.v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// All returns all configuration as a map.
func (c *Config) All() map[string]interface{} {
	return c.v.AllSettings()
}

// APIURL returns the configured API URL.
func (c *Config) APIURL() string {
	return c.GetString("api.url")
}

// PollInterval returns the poll interval as a duration.
func (c *Config) PollInterval() time.Duration {
	return c.parseDuration("worker.poll_interval", defaultPollIntervalDuration)
}

// HeartbeatInterval returns the heartbeat interval as a duration.
func (c *Config) HeartbeatInterval() time.Duration {
	return c.parseDuration("worker.heartbeat_interval", defaultHeartbeatIntervalDuration)
}

// HistoryEnabled returns whether transcript history is enabled.
func (c *Config) HistoryEnabled() bool {
	return c.v.GetBool("history.enabled")
}

// HistoryDir returns the configured transcript directory.
func (c *Config) HistoryDir() string {
	return c.GetString("history.dir")
}

// HistoryScrollbackLines returns the configured in-memory transcript ring size.
func (c *Config) HistoryScrollbackLines() int {
	return c.GetInt("history.scrollback_lines")
}

// parseDuration reads a config key and interprets it as a duration.
// It first tries time.ParseDuration (e.g. "30s", "1m"). If that fails,
// it tries parsing as a bare integer (seconds) for backward compatibility.
// Returns fallback if the result is less than minIntervalDuration.
func (c *Config) parseDuration(key string, fallback time.Duration) time.Duration {
	raw := c.GetString(key)
	if raw == "" {
		return fallback
	}

	// Try Go duration string first (e.g. "30s", "1m30s").
	if d, err := time.ParseDuration(raw); err == nil {
		if d < minIntervalDuration {
			return fallback
		}

		return d
	}

	// Backward compat: bare integer treated as seconds.
	if secs, err := strconv.Atoi(raw); err == nil {
		d := time.Duration(secs) * time.Second
		if d < minIntervalDuration {
			return fallback
		}

		return d
	}

	return fallback
}

// HistoryRetention returns the configured retention period for history pruning.
func (c *Config) HistoryRetention() time.Duration {
	d, err := time.ParseDuration(c.GetString("history.retention"))
	if err != nil || d <= 0 {
		return 30 * 24 * time.Hour
	}

	return d
}
