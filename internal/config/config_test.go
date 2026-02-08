package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Create a temporary directory without any config file
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Clear any environment variables that might interfere
	envVars := []string{"MUSH_API_URL", "MUSH_WORKER_POLL_INTERVAL", "MUSH_WORKER_HEARTBEAT_INTERVAL"}
	origValues := make(map[string]string)
	for _, env := range envVars {
		origValues[env] = os.Getenv(env)
		os.Unsetenv(env)
	}
	defer func() {
		for _, env := range envVars {
			if v := origValues[env]; v != "" {
				os.Setenv(env, v)
			}
		}
	}()

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
			want: DefaultPollInterval,
		},
		{
			name: "default heartbeat interval",
			accessor: func(c *Config) interface{} {
				return c.HeartbeatInterval()
			},
			want: DefaultHeartbeatInterval,
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
			// Save original and set test value
			orig := os.Getenv(tt.envVar)
			os.Setenv(tt.envVar, tt.envVal)
			defer func() {
				if orig != "" {
					os.Setenv(tt.envVar, orig)
				} else {
					os.Unsetenv(tt.envVar)
				}
			}()

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
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

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
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

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
			originalHome := os.Getenv("HOME")
			defer os.Setenv("HOME", originalHome)
			os.Setenv("HOME", tmpDir)

			orig := os.Getenv("MUSH_API_URL")
			if tt.envVal != "" {
				os.Setenv("MUSH_API_URL", tt.envVal)
			} else {
				os.Unsetenv("MUSH_API_URL")
			}
			defer func() {
				if orig != "" {
					os.Setenv("MUSH_API_URL", orig)
				} else {
					os.Unsetenv("MUSH_API_URL")
				}
			}()

			cfg := Load()
			got := cfg.APIURL()

			if got != tt.want {
				t.Errorf("APIURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_PollInterval(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   int
	}{
		{
			name:   "default",
			envVal: "",
			want:   DefaultPollInterval,
		},
		{
			name:   "from env",
			envVal: "45",
			want:   45,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			originalHome := os.Getenv("HOME")
			defer os.Setenv("HOME", originalHome)
			os.Setenv("HOME", tmpDir)

			orig := os.Getenv("MUSH_WORKER_POLL_INTERVAL")
			if tt.envVal != "" {
				os.Setenv("MUSH_WORKER_POLL_INTERVAL", tt.envVal)
			} else {
				os.Unsetenv("MUSH_WORKER_POLL_INTERVAL")
			}
			defer func() {
				if orig != "" {
					os.Setenv("MUSH_WORKER_POLL_INTERVAL", orig)
				} else {
					os.Unsetenv("MUSH_WORKER_POLL_INTERVAL")
				}
			}()

			cfg := Load()
			got := cfg.PollInterval()

			if got != tt.want {
				t.Errorf("PollInterval() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfig_HeartbeatInterval(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   int
	}{
		{
			name:   "default",
			envVal: "",
			want:   DefaultHeartbeatInterval,
		},
		{
			name:   "from env",
			envVal: "20",
			want:   20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			originalHome := os.Getenv("HOME")
			defer os.Setenv("HOME", originalHome)
			os.Setenv("HOME", tmpDir)

			orig := os.Getenv("MUSH_WORKER_HEARTBEAT_INTERVAL")
			if tt.envVal != "" {
				os.Setenv("MUSH_WORKER_HEARTBEAT_INTERVAL", tt.envVal)
			} else {
				os.Unsetenv("MUSH_WORKER_HEARTBEAT_INTERVAL")
			}
			defer func() {
				if orig != "" {
					os.Setenv("MUSH_WORKER_HEARTBEAT_INTERVAL", orig)
				} else {
					os.Unsetenv("MUSH_WORKER_HEARTBEAT_INTERVAL")
				}
			}()

			cfg := Load()
			got := cfg.HeartbeatInterval()

			if got != tt.want {
				t.Errorf("HeartbeatInterval() = %d, want %d", got, tt.want)
			}
		})
	}
}
