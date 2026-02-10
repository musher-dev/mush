// Package config handles Mush configuration using Viper.
//
// Configuration sources (in priority order):
//  1. Environment variables (MUSH_*)
//  2. Config file (~/.config/mush/config.yaml)
//  3. Built-in defaults
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	// DefaultAPIURL is the default Musher API endpoint.
	DefaultAPIURL = "https://api.musher.dev"
	// DefaultPollInterval is the default poll interval in seconds.
	DefaultPollInterval = 30
	// DefaultHeartbeatInterval is the default heartbeat interval in seconds.
	DefaultHeartbeatInterval = 30
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

	// Config file location
	home, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(home, ".config", "mush")
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
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config file: %v\n", err)
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
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".config", "mush")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}

	configFile := filepath.Join(configDir, "config.yaml")
	return c.v.WriteConfigAs(configFile)
}

// All returns all configuration as a map.
func (c *Config) All() map[string]interface{} {
	return c.v.AllSettings()
}

// APIURL returns the configured API URL.
func (c *Config) APIURL() string {
	return c.GetString("api.url")
}

// PollInterval returns the poll interval in seconds.
func (c *Config) PollInterval() int {
	return c.GetInt("worker.poll_interval")
}

// HeartbeatInterval returns the heartbeat interval in seconds.
func (c *Config) HeartbeatInterval() int {
	return c.GetInt("worker.heartbeat_interval")
}
