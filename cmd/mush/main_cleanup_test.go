package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWrapNamedPostRunCleanup_ErrorIncludesCleanupName(t *testing.T) {
	wrapped := wrapNamedPostRunCleanup(nil, "telemetry resources", func() error {
		return errors.New("boom")
	})

	err := wrapped(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "cleanup telemetry resources") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWrapPostRunCleanup_UsesLoggerResourcesLabel(t *testing.T) {
	wrapped := wrapPostRunCleanup(nil, func() error {
		return errors.New("boom")
	})

	err := wrapped(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "cleanup logger resources") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWrapNamedPostRunCleanup_CleansUpWhenPostRunFails(t *testing.T) {
	cleanupCalled := false
	postErr := errors.New("post-run failed")
	wrapped := wrapNamedPostRunCleanup(
		func(*cobra.Command, []string) error {
			return postErr
		},
		"telemetry resources",
		func() error {
			cleanupCalled = true
			return nil
		},
	)

	err := wrapped(&cobra.Command{}, nil)
	if !errors.Is(err, postErr) {
		t.Fatalf("expected post-run error, got %v", err)
	}

	if !cleanupCalled {
		t.Fatal("expected cleanup to be called when post-run fails")
	}
}
