//go:build unix

package main

import (
	"io"
	"strings"
	"testing"

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
