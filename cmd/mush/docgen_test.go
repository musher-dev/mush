package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
)

// docsDir is the output directory for generated CLI reference markdown.
const docsDir = "../../docs/reference/cli"

// TestGenerateCLIDocs generates markdown reference docs from the Cobra command
// tree and compares them against the committed files in docs/reference/cli/.
// Set UPDATE_GOLDEN=1 to write files instead of comparing.
func TestGenerateCLIDocs(t *testing.T) {
	root := newRootCmd()

	// Disable auto-gen tags for deterministic output.
	for _, cmd := range collectAllCommands(root) {
		cmd.DisableAutoGenTag = true
	}

	// Generate all markdown files.
	generated := make(map[string]string) // filename → content

	for _, cmd := range collectAllCommands(root) {
		if !cmd.IsAvailableCommand() && cmd != root {
			continue
		}

		filename := cmdFilename(cmd)

		var buf bytes.Buffer

		// Prepend frontmatter.
		fmt.Fprintf(&buf, "---\ntitle: %q\ndescription: %q\n---\n\n", cmd.CommandPath(), cmd.Short)

		if err := doc.GenMarkdownCustom(cmd, &buf, linkHandler); err != nil {
			t.Fatalf("GenMarkdownCustom(%s): %v", cmd.CommandPath(), err)
		}

		// Append hidden flags section for commands that define hidden flags.
		if section := renderHiddenFlagsSection(cmd); section != "" {
			content := buf.String()

			// Insert before "### SEE ALSO" if present, otherwise append.
			if idx := strings.Index(content, "### SEE ALSO"); idx != -1 {
				buf.Reset()
				buf.WriteString(content[:idx])
				buf.WriteString(section)
				buf.WriteString(content[idx:])
			} else {
				buf.WriteString(section)
			}
		}

		generated[filename] = buf.String()
	}

	// Generate index README.md.
	generated["README.md"] = generateIndex(root)

	update := os.Getenv("UPDATE_GOLDEN") != ""

	if update {
		absDir, err := filepath.Abs(docsDir)
		if err != nil {
			t.Fatalf("resolve docs dir: %v", err)
		}

		if err := os.MkdirAll(absDir, 0o755); err != nil {
			t.Fatalf("create docs dir: %v", err)
		}

		for filename, content := range generated {
			path := filepath.Join(absDir, filename)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}

			t.Logf("wrote %s", path)
		}

		// Remove stale files.
		removeStaleFiles(t, absDir, generated)

		return
	}

	// Compare mode: check each generated file against committed file.
	absDir, err := filepath.Abs(docsDir)
	if err != nil {
		t.Fatalf("resolve docs dir: %v", err)
	}

	for filename, want := range generated {
		path := filepath.Join(absDir, filename)

		got, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("missing file %s; run UPDATE_GOLDEN=1 go test ./cmd/mush -run TestGenerateCLIDocs to generate", filename)

				continue
			}

			t.Fatalf("read %s: %v", path, err)
		}

		if string(got) != want {
			t.Errorf("docs drift in %s; run UPDATE_GOLDEN=1 go test ./cmd/mush -run TestGenerateCLIDocs to regenerate", filename)
		}
	}

	// Check for stale files.
	checkStaleFiles(t, absDir, generated)
}

// cmdFilename returns the markdown filename for a command (e.g. "mush_worker_start.md").
func cmdFilename(cmd *cobra.Command) string {
	name := strings.ReplaceAll(cmd.CommandPath(), " ", "_")
	return name + ".md"
}

// linkHandler rewrites cobra doc links to relative .md links (GitHub-browsable).
func linkHandler(link string) string {
	return link
}

// generateIndex creates the README.md index grouped by command group.
func generateIndex(root *cobra.Command) string {
	var buf bytes.Buffer

	buf.WriteString("# CLI Reference\n\n")
	buf.WriteString("> Auto-generated from source. Do not edit manually.\n")
	buf.WriteString("> Run `task docs:generate` to regenerate.\n\n")

	// Group definitions in display order.
	groups := []struct {
		id    string
		title string
	}{
		{"core", "Core Commands"},
		{"account", "Account & Configuration"},
		{"setup", "Setup & Diagnostics"},
	}

	// Root command link.
	fmt.Fprintf(&buf, "- [%s](%s) — %s\n\n", root.CommandPath(), cmdFilename(root), root.Short)

	for _, g := range groups {
		fmt.Fprintf(&buf, "## %s\n\n", g.title)

		var cmds []*cobra.Command

		for _, cmd := range collectAllCommands(root) {
			if cmd == root || !cmd.IsAvailableCommand() {
				continue
			}

			if cmd.GroupID == g.id || (cmd.Parent() != nil && cmd.Parent() != root && cmd.Parent().GroupID == g.id) {
				cmds = append(cmds, cmd)
			}
		}

		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].CommandPath() < cmds[j].CommandPath()
		})

		for _, cmd := range cmds {
			indent := ""
			if cmd.Parent() != nil && cmd.Parent() != root {
				indent = "  "
			}

			fmt.Fprintf(&buf, "%s- [%s](%s) — %s\n", indent, cmd.CommandPath(), cmdFilename(cmd), cmd.Short)
		}

		buf.WriteString("\n")
	}

	return buf.String()
}

// removeStaleFiles removes .md files in dir that are not in the generated set.
func removeStaleFiles(t *testing.T, dir string, generated map[string]string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		if _, ok := generated[e.Name()]; !ok {
			path := filepath.Join(dir, e.Name())
			if err := os.Remove(path); err != nil {
				t.Errorf("remove stale file %s: %v", path, err)
			} else {
				t.Logf("removed stale file: %s", e.Name())
			}
		}
	}
}

// checkStaleFiles fails the test if there are .md files not in the generated set.
func checkStaleFiles(t *testing.T, dir string, generated map[string]string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		if _, ok := generated[e.Name()]; !ok {
			t.Errorf("stale file %s in %s; remove it or run UPDATE_GOLDEN=1 to clean up", e.Name(), dir)
		}
	}
}

// renderHiddenFlagsSection returns a markdown section listing hidden flags
// defined directly on cmd (not inherited). Returns "" if no hidden flags exist.
func renderHiddenFlagsSection(cmd *cobra.Command) string {
	// Collect hidden flags defined on this command (not inherited).
	var hidden []*pflag.Flag

	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			hidden = append(hidden, f)
		}
	})

	if len(hidden) == 0 {
		return ""
	}

	// Build a temporary FlagSet with the hidden flags unhidden so
	// FlagUsagesWrapped renders them.
	fs := pflag.NewFlagSet("hidden", pflag.ContinueOnError)

	for _, f := range hidden {
		clone := *f
		clone.Hidden = false
		fs.AddFlag(&clone)
	}

	var buf bytes.Buffer

	buf.WriteString("### Hidden Flags\n\n")
	buf.WriteString("These flags are omitted from `--help` but remain fully functional.\n")
	buf.WriteString("They can also be set via environment variables (`MUSH_LOG_LEVEL`, etc.).\n\n")
	buf.WriteString("```\n")
	buf.WriteString(strings.TrimRight(fs.FlagUsagesWrapped(0), "\n"))
	buf.WriteString("\n```\n\n")

	return buf.String()
}
