//go:build unix

package main

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
	"github.com/musher-dev/mush/internal/tui/nav"
)

func TestHandleBareRunNavResultRequiresTTY(t *testing.T) {
	t.Parallel()

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: false})
	result := &nav.Result{Action: nav.ActionBareRun, Harness: "claude"}

	err := handleBareRunNavResult(&cobra.Command{}, out, result)
	if err == nil {
		t.Fatal("expected error for non-tty launch")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("code = %d, want %d", cliErr.Code, clierrors.ExitUsage)
	}
}

func TestHandleBareRunNavResultRequiresHarness(t *testing.T) {
	t.Parallel()

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: true})
	result := &nav.Result{Action: nav.ActionBareRun}

	err := handleBareRunNavResult(&cobra.Command{}, out, result)
	if err == nil {
		t.Fatal("expected error for missing harness")
	}

	if !strings.Contains(err.Error(), "Harness type is required") {
		t.Fatalf("error = %q, want missing harness message", err.Error())
	}
}

func TestHandleBundleLoadNavResultRequiresBundleInfo(t *testing.T) {
	t.Parallel()

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: true})
	result := &nav.Result{
		Action:     nav.ActionBundleLoad,
		Harness:    "claude",
		BundleSlug: "readme-maker",
	}

	err := handleBundleLoadNavResult(&cobra.Command{}, out, result)
	if err == nil {
		t.Fatal("expected error for missing bundle information")
	}

	if !strings.Contains(err.Error(), "Missing bundle information") {
		t.Fatalf("error = %q, want missing bundle information message", err.Error())
	}
}

func TestHandleBundleInstallNavResultRequiresBundleInfo(t *testing.T) {
	t.Parallel()

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: true})
	result := &nav.Result{
		Action:     nav.ActionBundleInstall,
		Harness:    "claude",
		BundleSlug: "test-bundle",
		BundleVer:  "1.0.0",
	}

	err := handleBundleInstallNavResult(&cobra.Command{}, out, result)
	if err == nil {
		t.Fatal("expected error for missing bundle information")
	}

	if !strings.Contains(err.Error(), "Missing bundle information") {
		t.Fatalf("error = %q, want missing bundle information message", err.Error())
	}
}

func TestHandleBundleInstallNavResultRequiresHarness(t *testing.T) {
	t.Parallel()

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: true})
	result := &nav.Result{
		Action:          nav.ActionBundleInstall,
		BundleNamespace: "acme",
		BundleSlug:      "test",
		BundleVer:       "1.0.0",
	}

	err := handleBundleInstallNavResult(&cobra.Command{}, out, result)
	if err == nil {
		t.Fatal("expected error for missing harness")
	}

	if !strings.Contains(err.Error(), "Harness type is required") {
		t.Fatalf("error = %q, want missing harness message", err.Error())
	}
}
