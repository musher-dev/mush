package config

import (
	"os"
	"testing"
	"time"
)

// unsetEnvForTest unsets an environment variable and registers cleanup to
// restore its original state (including distinguishing "unset" from "set to
// empty string").
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	os.Unsetenv(key)
}

func TestLoad_Defaults(t *testing.T) {
	// Create a temporary directory without any config file
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Clear any environment variables that might interfere
	unsetEnvForTest(t, "MUSH_API_URL")
	unsetEnvForTest(t, "MUSH_WORKER_POLL_INTERVAL")
	unsetEnvForTest(t, "MUSH_WORKER_HEARTBEAT_INTERVAL")

	cfg := Load()

	tests := []struct {
		name     string
		got      interface{}
		want     interface{}
		accessor func(*Config) interface{}
	}{
		{
			name: "default API URL",
			accessor: func(c *Config) interface{} {
				return c.APIURL()
			},
			want: DefaultAPIURL,
		},
		{
			name: "default poll interval",
			accessor: func(c *Config) interface{} {
				return c.PollInterval()
			},
			want: 30 * time.Second,
		},
		{
			name: "default heartbeat interval",
			accessor: func(c *Config) interface{} {
				return c.HeartbeatInterval()
			},
			want: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.accessor(cfg)
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoad_FromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		envVal  string
		key     string
		wantStr string
		wantInt int
	}{
		{
			name:    "API URL from env",
			envVar:  "MUSH_API_URL",
			envVal:  "https://custom.api.com",
			key:     "api.url",
			wantStr: "https://custom.api.com",
		},
		{
			name:    "poll interval from env",
			envVar:  "MUSH_WORKER_POLL_INTERVAL",
			envVal:  "60",
			key:     "worker.poll_interval",
			wantInt: 60,
		},
		{
			name:    "heartbeat interval from env",
			envVar:  "MUSH_WORKER_HEARTBEAT_INTERVAL",
			envVal:  "15",
			key:     "worker.heartbeat_interval",
			wantInt: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.envVal)

			cfg := Load()

			if tt.wantStr != "" {
				got := cfg.GetString(tt.key)
				if got != tt.wantStr {
					t.Errorf("GetString(%q) = %q, want %q", tt.key, got, tt.wantStr)
				}
			}

			if tt.wantInt != 0 {
				got := cfg.GetInt(tt.key)
				if got != tt.wantInt {
					t.Errorf("GetInt(%q) = %d, want %d", tt.key, got, tt.wantInt)
				}
			}
		})
	}
}

func TestConfig_All(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	unsetEnvForTest(t, "MUSH_API_URL")
	unsetEnvForTest(t, "MUSH_WORKER_POLL_INTERVAL")
	unsetEnvForTest(t, "MUSH_WORKER_HEARTBEAT_INTERVAL")

	cfg := Load()
	all := cfg.All()

	if all == nil {
		t.Fatal("All() returned nil")
	}

	// Check that defaults are present
	if _, ok := all["api"]; !ok {
		t.Error("All() missing 'api' key")
	}

	if _, ok := all["worker"]; !ok {
		t.Error("All() missing 'worker' key")
	}
}

func TestConfig_Get(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	unsetEnvForTest(t, "MUSH_API_URL")

	cfg := Load()

	// Get should work for nested keys
	got := cfg.Get("api.url")
	if got == nil {
		t.Error("Get(\"api.url\") returned nil")
	}

	str, ok := got.(string)
	if !ok {
		t.Errorf("Get(\"api.url\") type = %T, want string", got)
	}

	if str != DefaultAPIURL {
		t.Errorf("Get(\"api.url\") = %q, want %q", str, DefaultAPIURL)
	}
}

func TestConfig_APIURL(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{
			name:   "default",
			envVal: "",
			want:   DefaultAPIURL,
		},
		{
			name:   "from env",
			envVal: "https://api.example.com",
			want:   "https://api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			if tt.envVal != "" {
				t.Setenv("MUSH_API_URL", tt.envVal)
			} else {
				unsetEnvForTest(t, "MUSH_API_URL")
			}

			cfg := Load()
			got := cfg.APIURL()

			if got != tt.want {
				t.Errorf("APIURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func runDurationConfigCase(t *testing.T, envKey, envValue string, getter func(*Config) time.Duration) time.Duration {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if envValue != "" {
		t.Setenv(envKey, envValue)
	} else {
		unsetEnvForTest(t, envKey)
	}

	cfg := Load()

	return getter(cfg)
}

func TestConfig_PollInterval(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   time.Duration
	}{
		{
			name:   "default",
			envVal: "",
			want:   30 * time.Second,
		},
		{
			name:   "duration string from env",
			envVal: "45s",
			want:   45 * time.Second,
		},
		{
			name:   "bare integer from env (backward compat)",
			envVal: "60",
			want:   60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runDurationConfigCase(t, "MUSH_WORKER_POLL_INTERVAL", tt.envVal, func(cfg *Config) time.Duration {
				return cfg.PollInterval()
			})

			if got != tt.want {
				t.Errorf("PollInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_HeartbeatInterval(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   time.Duration
	}{
		{
			name:   "default",
			envVal: "",
			want:   30 * time.Second,
		},
		{
			name:   "duration string from env",
			envVal: "20s",
			want:   20 * time.Second,
		},
		{
			name:   "bare integer from env (backward compat)",
			envVal: "15",
			want:   15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runDurationConfigCase(t, "MUSH_WORKER_HEARTBEAT_INTERVAL", tt.envVal, func(cfg *Config) time.Duration {
				return cfg.HeartbeatInterval()
			})

			if got != tt.want {
				t.Errorf("HeartbeatInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}
