package main

import (
	"os"
	"strings"
	"testing"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func TestValidateAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "https valid", raw: "https://api.example.dev"},
		{name: "http valid", raw: "http://localhost:8080"},
		{name: "trims spaces", raw: "  https://api.example.dev  "},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "no scheme", raw: "api.example.dev", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://api.example.dev", wantErr: true},
		{name: "missing host", raw: "https:///path", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateAPIURL(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateAPIURL(%q) expected error", tc.raw)
				}

				return
			}

			if err != nil {
				t.Fatalf("validateAPIURL(%q) error = %v", tc.raw, err)
			}

			if got == "" {
				t.Fatal("validated URL must not be empty")
			}
		})
	}
}

func TestRootCmd_APIURLFlagSetsEnv(t *testing.T) {
	t.Setenv("MUSH_API_URL", "https://from-env.example")

	root := newRootCmd()
	root.SetArgs([]string{"--api-url", "https://from-flag.example", "version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("root.Execute() error = %v", err)
	}

	if got := strings.TrimSpace(os.Getenv("MUSH_API_URL")); got != "https://from-flag.example" {
		t.Fatalf("MUSH_API_URL = %q, want https://from-flag.example", got)
	}
}

func TestRootCmd_APIURLFlagRejectsInvalidValue(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"--api-url", "bad-url", "version"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --api-url")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("exit code = %d, want %d", cliErr.Code, clierrors.ExitUsage)
	}

	if !strings.Contains(cliErr.Message, "Invalid API URL") {
		t.Fatalf("error message = %q, want Invalid API URL", cliErr.Message)
	}
}
