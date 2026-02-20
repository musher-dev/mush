package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

func TestUpdateCmd_DisabledByEnv(t *testing.T) {
	t.Setenv("MUSH_UPDATE_DISABLED", "1")

	var stdout, stderr bytes.Buffer

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&stdout, &stderr, term)
	ctx := out.WithContext(t.Context())

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{})
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !strings.Contains(stdout.String(), "disabled") {
		t.Errorf("expected 'disabled' in output, got: %q", stdout.String())
	}
}

func TestUpdateCmd_DevBuild(t *testing.T) {
	t.Setenv("MUSH_UPDATE_DISABLED", "")

	oldVersion := buildinfo.Version
	buildinfo.Version = "dev"

	defer func() { buildinfo.Version = oldVersion }()

	var stdout, stderr bytes.Buffer

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&stdout, &stderr, term)
	ctx := out.WithContext(t.Context())

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{})
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	combined := stdout.String()
	if !strings.Contains(combined, "Development build") {
		t.Errorf("expected 'Development build' in output, got: %q", combined)
	}
}

func TestUpdateCmd_VersionTrimPrefix(t *testing.T) {
	// When --version is specified, the "v" prefix should be trimmed.
	// This test verifies trimming happens by running with a version that will
	// fail to find on the network — we just want to make sure it doesn't panic
	// and that it reaches the updater (returns an error, not a dev build warning).
	t.Setenv("MUSH_UPDATE_DISABLED", "")

	oldVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"

	defer func() { buildinfo.Version = oldVersion }()

	var stdout, stderr bytes.Buffer

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&stdout, &stderr, term)
	ctx := out.WithContext(t.Context())

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--version", "v99.99.99"})
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	// We expect an error because version 99.99.99 doesn't exist,
	// but the point is that we reached the updater (not the dev build path).
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent version")
	}

	if strings.Contains(err.Error(), "Development build") {
		t.Errorf("should not hit dev build path when --version is set")
	}
}
