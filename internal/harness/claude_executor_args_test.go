//go:build unix

package harness

import "testing"

func TestClaudeCommandArgs_DefaultMode(t *testing.T) {
	exec := NewClaudeExecutor()

	exec.opts = SetupOptions{
		BundleDir: "/tmp/bundle",
	}
	exec.mcpConfigPath = "/tmp/mcp.json"

	got := exec.commandArgs()
	want := []string{
		"--dangerously-skip-permissions",
		"--add-dir", "/tmp/bundle",
		"--mcp-config", "/tmp/mcp.json",
	}

	assertStringSliceEqual(t, got, want)
}

func TestClaudeCommandArgs_BundleLoadModeOmitsSkipPermissions(t *testing.T) {
	exec := NewClaudeExecutor()

	exec.opts = SetupOptions{
		BundleLoadMode: true,
		BundleDir:      "/tmp/bundle",
	}
	exec.mcpConfigPath = "/tmp/mcp.json"

	got := exec.commandArgs()
	want := []string{
		"--add-dir", "/tmp/bundle",
		"--mcp-config", "/tmp/mcp.json",
	}

	assertStringSliceEqual(t, got, want)
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(args) = %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q: got=%v want=%v", i, got[i], want[i], got, want)
		}
	}
}
