package main

import (
	"io"
	"testing"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

func TestConfigureRootRuntime_MUSHJSONSuppressesUpdateChecks(t *testing.T) {
	t.Setenv("MUSH_JSON", "1")

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: false})
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(t.Context())

	state, err := configureRootRuntime(cmd, out, false, false, false, false, "", "", "", "")
	if err != nil {
		t.Fatalf("configureRootRuntime() error = %v", err)
	}

	if !state.out.JSON {
		t.Fatal("expected JSON mode to be enabled from environment")
	}

	if shouldBackgroundCheck(cmd, "1.2.3", state.out) {
		t.Fatal("expected background update check to be suppressed in JSON mode")
	}

	if shouldShowUpdateNotice(cmd, "1.2.3", state.out) {
		t.Fatal("expected update notice to be suppressed in JSON mode")
	}
}

func TestConfigureRootRuntime_MUSHQuietSuppressesUpdateChecks(t *testing.T) {
	t.Setenv("MUSH_QUIET", "1")

	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{IsTTY: false})
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(t.Context())

	state, err := configureRootRuntime(cmd, out, false, false, false, false, "", "", "", "")
	if err != nil {
		t.Fatalf("configureRootRuntime() error = %v", err)
	}

	if !state.out.Quiet {
		t.Fatal("expected quiet mode to be enabled from environment")
	}

	if shouldBackgroundCheck(cmd, "1.2.3", state.out) {
		t.Fatal("expected background update check to be suppressed in quiet mode")
	}

	if shouldShowUpdateNotice(cmd, "1.2.3", state.out) {
		t.Fatal("expected update notice to be suppressed in quiet mode")
	}
}
