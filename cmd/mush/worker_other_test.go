//go:build !unix

package main

import (
	"strings"
	"testing"

	clierrors "github.com/musher-dev/mush/internal/errors"
)

func TestWorkerStartUnsupportedOnNonUnix(t *testing.T) {
	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for worker start on non-unix")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("error code = %d, want %d (ExitUsage)", cliErr.Code, clierrors.ExitUsage)
	}

	if !strings.Contains(strings.ToLower(cliErr.Message), "not supported") {
		t.Fatalf("error message = %q, want unsupported message", cliErr.Message)
	}
}
