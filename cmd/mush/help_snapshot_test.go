package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/testutil"
)

// TestHelpSnapshots captures the --help output of every command and compares
// against golden files. This catches accidental regressions to flag
// descriptions, help formatting, and command ordering.
//
// Run UPDATE_GOLDEN=1 go test ./cmd/mush -run TestHelpSnapshots to refresh.
func TestHelpSnapshots(t *testing.T) {
	// Collect command paths from a single root instance.
	root := newRootCmd()

	var paths []string

	for _, cmd := range collectAllCommands(root) {
		if !cmd.IsAvailableCommand() && cmd != root {
			continue
		}

		paths = append(paths, cmd.CommandPath())
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			// Create a fresh root per command to avoid flag-already-registered panics.
			freshRoot := newRootCmd()

			// Build args: subcommand parts + --help, dispatched through root.
			parts := strings.Fields(path)
			args := make([]string, 0, len(parts))
			args = append(args, parts[1:]...)
			args = append(args, "--help")

			var buf bytes.Buffer
			freshRoot.SetOut(&buf)
			freshRoot.SetErr(&buf)
			freshRoot.SetArgs(args)

			if err := freshRoot.Execute(); err != nil {
				// Help should not error, but capture output anyway.
				t.Logf("help returned error: %v", err)
			}

			goldenFile := "help_snapshots/" + strings.ReplaceAll(path, " ", "_") + ".help"
			testutil.AssertGolden(t, buf.String(), goldenFile)
		})
	}
}
