//go:build unix

package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

func TestBundleLoadRequiresHarness(t *testing.T) {
	term := &terminal.Info{IsTTY: true}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	cmd := newBundleLoadCmd()
	cmd.SetArgs([]string{"my-bundle"})
	cmd.SetContext(out.WithContext(t.Context()))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --harness is missing")
	}

	if !strings.Contains(err.Error(), `required flag(s) "harness" not set`) {
		t.Fatalf("error = %q, want required harness flag error", err.Error())
	}
}

func TestBundleLoadRequiresTTY(t *testing.T) {
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	cmd := newBundleLoadCmd()
	cmd.SetArgs([]string{"my-bundle", "--harness", "bash"})
	cmd.SetContext(out.WithContext(t.Context()))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected TTY error")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("error code = %d, want %d", cliErr.Code, clierrors.ExitUsage)
	}

	if !strings.Contains(cliErr.Message, "TTY") {
		t.Fatalf("error message = %q, want to contain TTY", cliErr.Message)
	}
}

func withMockTryAPIClient(t *testing.T, source auth.CredentialSource, c *client.Client, wsOverride string) {
	t.Helper()

	prev := tryAPIClient
	tryAPIClient = func() (auth.CredentialSource, *client.Client, string, error) {
		return source, c, wsOverride, nil
	}

	t.Cleanup(func() { tryAPIClient = prev })
}

func TestBundleInstallAnonymousFallbackShowsAuthHint(t *testing.T) {
	// Mock tryAPIClient to return an anonymous client whose transport returns 403.
	hc := &http.Client{
		Transport: linkRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"detail":"forbidden"}`)),
			}, nil
		}),
	}

	anonClient := client.NewWithHTTPClient("https://api.test", "", hc)
	withMockTryAPIClient(t, auth.SourceNone, anonClient, "public")

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	cmd := newBundleInstallCmd()
	cmd.SetArgs([]string{"private-bundle:1.0.0", "--harness", "claude"})
	cmd.SetContext(out.WithContext(t.Context()))

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for private bundle with anonymous client")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitAuth {
		t.Fatalf("error code = %d, want %d (ExitAuth)", cliErr.Code, clierrors.ExitAuth)
	}

	if !strings.Contains(cliErr.Hint, "mush auth login") {
		t.Fatalf("hint = %q, want to contain 'mush auth login'", cliErr.Hint)
	}
}

func TestBundleCommandHasNoRunSubcommand(t *testing.T) {
	cmd := newBundleCmd()

	hasLoad := false

	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "load":
			hasLoad = true
		case "run":
			t.Fatal("unexpected legacy subcommand: run")
		}
	}

	if !hasLoad {
		t.Fatal("expected load subcommand")
	}
}
