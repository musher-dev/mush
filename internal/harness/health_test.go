package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckHealth_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := CheckHealth(t.Context(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestCheckAllHealth(t *testing.T) {
	t.Parallel()

	reports := CheckAllHealth(t.Context())
	if len(reports) < 2 {
		t.Fatalf("expected at least 2 reports, got %d", len(reports))
	}

	for _, r := range reports {
		if r.ProviderName == "" {
			t.Error("report has empty ProviderName")
		}

		if r.DisplayName == "" {
			t.Error("report has empty DisplayName")
		}

		if len(r.Results) == 0 {
			t.Errorf("report for %s has no results", r.ProviderName)
		}

		// Binary check should always be present.
		if r.Results[0].Check != "Binary" {
			t.Errorf("first check for %s should be Binary, got %s", r.ProviderName, r.Results[0].Check)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"~/.config/bar", filepath.Join(home, ".config", "bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		got := expandTilde(tt.input)
		if got != tt.want {
			t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCheckConfigDir_TempDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	spec := &ProviderSpec{
		Binary: "echo",
		Status: &StatusSpec{
			ConfigDir: dir,
		},
	}

	result := checkConfigDir(spec)

	if result.Status != HealthPass {
		t.Errorf("status = %d, want HealthPass for existing dir", result.Status)
	}

	if result.Check != "Config" {
		t.Errorf("check = %q, want Config", result.Check)
	}
}

func TestCheckConfigDir_Missing(t *testing.T) {
	t.Parallel()

	spec := &ProviderSpec{
		Binary: "echo",
		Status: &StatusSpec{
			ConfigDir: "/nonexistent/path/that/does/not/exist",
		},
	}

	result := checkConfigDir(spec)

	if result.Status != HealthWarn {
		t.Errorf("status = %d, want HealthWarn for missing dir", result.Status)
	}
}

func TestCheckAuthFile_Missing(t *testing.T) {
	t.Parallel()

	spec := &ProviderSpec{
		Binary: "echo",
		Status: &StatusSpec{
			AuthCheck: &AuthCheck{
				Path:        "/nonexistent/credentials.json",
				Description: "Credentials",
			},
		},
	}

	result := checkAuthFile(spec)

	if result.Status != HealthWarn {
		t.Errorf("status = %d, want HealthWarn for missing auth file", result.Status)
	}

	if result.Check != "Credentials" {
		t.Errorf("check = %q, want Credentials", result.Check)
	}
}

func TestCheckAuthFile_Exists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials.json")

	if err := os.WriteFile(credFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	spec := &ProviderSpec{
		Binary: "echo",
		Status: &StatusSpec{
			AuthCheck: &AuthCheck{
				Path:        credFile,
				Description: "Credentials",
			},
		},
	}

	result := checkAuthFile(spec)

	if result.Status != HealthPass {
		t.Errorf("status = %d, want HealthPass for existing auth file", result.Status)
	}
}

func TestCheckAuthFile_DefaultDescription(t *testing.T) {
	t.Parallel()

	spec := &ProviderSpec{
		Binary: "echo",
		Status: &StatusSpec{
			AuthCheck: &AuthCheck{
				Path: "/nonexistent/file",
			},
		},
	}

	result := checkAuthFile(spec)

	if result.Check != "Auth" {
		t.Errorf("check = %q, want Auth (default description)", result.Check)
	}
}
