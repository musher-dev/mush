// Package config handles Mush configuration using Viper.
//
// Configuration sources (in priority order):
//  1. Environment variables (MUSHER_*)
//  2. Config file (<user config dir>/musher/config.yaml)
//  3. Built-in defaults
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	// DefaultUpdateCheckInterval is the default background update check interval.
	DefaultUpdateCheckInterval = "24h"
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
	v.SetDefault("network.ca_cert_file", "")
	v.SetDefault("tui", true)
	v.SetDefault("history.enabled", true)
	v.SetDefault("history.scrollback_lines", 10000)
	v.SetDefault("history.retention", (30 * 24 * time.Hour).String())
	v.SetDefault("update.auto_apply", true)
	v.SetDefault("update.check_interval", DefaultUpdateCheckInterval)
	v.SetDefault("harness.scrollback_lines", 1000)
	v.SetDefault("experimental", false)

	// Config file location
	configDir, err := paths.ConfigRoot()
	if err == nil {
		historyDir, historyErr := paths.HistoryDir()
		if historyErr == nil {
			v.SetDefault("history.dir", historyDir)
		} else {
			if home, homeErr := os.UserHomeDir(); homeErr == nil {
				v.SetDefault("history.dir", filepath.Join(home, ".local", "state", "musher", "history"))
			}
		}

		v.AddConfigPath(configDir)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Environment variables
	v.SetEnvPrefix("MUSHER")
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

// CACertFile returns the optional custom CA certificate bundle path.
func (c *Config) CACertFile() string {
	return strings.TrimSpace(c.GetString("network.ca_cert_file"))
}

// PollInterval returns the poll interval as a duration.
func (c *Config) PollInterval() time.Duration {
	return c.parseDuration("worker.poll_interval", defaultPollIntervalDuration)
}

// HeartbeatInterval returns the heartbeat interval as a duration.
func (c *Config) HeartbeatInterval() time.Duration {
	return c.parseDuration("worker.heartbeat_interval", defaultHeartbeatIntervalDuration)
}

// TUI returns whether the interactive TUI is enabled.
func (c *Config) TUI() bool {
	return c.v.GetBool("tui")
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
// It tries time.ParseDuration (e.g. "30s", "1m").
// Returns fallback if the value is empty, unparseable, or less than minIntervalDuration.
func (c *Config) parseDuration(key string, fallback time.Duration) time.Duration {
	raw := c.GetString(key)
	if raw == "" {
		return fallback
	}

	if d, err := time.ParseDuration(raw); err == nil {
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

// HarnessScrollbackLines returns the configured scrollback buffer capacity for the harness TUI.
func (c *Config) HarnessScrollbackLines() int {
	return c.GetInt("harness.scrollback_lines")
}

// Experimental returns whether experimental features are enabled.
func (c *Config) Experimental() bool {
	return c.v.GetBool("experimental")
}

// UpdateAutoApply returns whether background auto-apply is enabled.
func (c *Config) UpdateAutoApply() bool {
	return c.v.GetBool("update.auto_apply")
}

// UpdateCheckInterval returns the configured background update check interval.
func (c *Config) UpdateCheckInterval() time.Duration {
	return c.parseDuration("update.check_interval", 24*time.Hour)
}
