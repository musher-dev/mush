package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/pflag"
)

// TestAllRunnableCommandsHaveExample walks the entire command tree and fails if
// any runnable command is missing an Example field. This prevents future
// commands from shipping without usage examples.
func TestAllRunnableCommandsHaveExample(t *testing.T) {
	root := newRootCmd()

	var missing []string

	for _, cmd := range collectAllCommands(root) {
		if !cmd.Runnable() {
			continue
		}

		if strings.TrimSpace(cmd.Example) == "" {
			missing = append(missing, cmd.CommandPath())
		}
	}

	if len(missing) > 0 {
		t.Errorf("runnable commands missing Example field:\n  %s\n\nAdd Example: `  mush <cmd> ...` to each command.",
			strings.Join(missing, "\n  "))
	}
}

// TestAllRunnableCommandsHaveLong walks the entire command tree and fails if
// any runnable command is missing a Long description. This prevents future
// commands from shipping without a detailed help description.
func TestAllRunnableCommandsHaveLong(t *testing.T) {
	root := newRootCmd()

	var missing []string

	for _, cmd := range collectAllCommands(root) {
		if !cmd.Runnable() {
			continue
		}

		if strings.TrimSpace(cmd.Long) == "" {
			missing = append(missing, cmd.CommandPath())
		}
	}

	if len(missing) > 0 {
		t.Errorf("runnable commands missing Long description:\n  %s\n\nAdd a Long field with 1-2 sentences explaining the command.",
			strings.Join(missing, "\n  "))
	}
}

// TestLongDescriptionNoEmbeddedExamples checks that no Long description
// contains embedded examples (which should go in the Example field instead).
func TestLongDescriptionNoEmbeddedExamples(t *testing.T) {
	root := newRootCmd()

	var violations []string

	for _, cmd := range collectAllCommands(root) {
		long := cmd.Long
		if long == "" {
			continue
		}

		if strings.Contains(long, "Example:") || strings.Contains(long, "```") {
			violations = append(violations, cmd.CommandPath())
		}
	}

	if len(violations) > 0 {
		t.Errorf("commands with examples embedded in Long (move to Example field):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// TestForceFlagsHaveShortF checks that every --force flag in the command tree
// has -f as a shorthand.
func TestForceFlagsHaveShortF(t *testing.T) {
	root := newRootCmd()

	var missing []string

	for _, cmd := range collectAllCommands(root) {
		f := cmd.Flags().Lookup("force")
		if f == nil {
			continue
		}

		if f.Shorthand != "f" {
			missing = append(missing, cmd.CommandPath())
		}
	}

	if len(missing) > 0 {
		t.Errorf("--force flags missing -f shorthand:\n  %s\n\nUse BoolVarP with \"f\" shorthand.",
			strings.Join(missing, "\n  "))
	}
}

// TestShortDescriptionsAreConcise checks that all Short descriptions are
// 60 characters or fewer, preventing overly long one-liners in help output.
func TestShortDescriptionsAreConcise(t *testing.T) {
	const maxLen = 60

	root := newRootCmd()

	var violations []string

	for _, cmd := range collectAllCommands(root) {
		short := cmd.Short
		if short == "" {
			continue
		}

		if len(short) > maxLen {
			violations = append(violations, fmt.Sprintf("%s (%d chars): %q", cmd.CommandPath(), len(short), short))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Short descriptions exceeding %d characters:\n  %s\n\nKeep Short fields concise — use Long for details.",
			maxLen, strings.Join(violations, "\n  "))
	}
}

// TestShortDescriptionsStyle checks that Short descriptions start with an
// uppercase letter and do not end with a period, following Cobra conventions.
func TestShortDescriptionsStyle(t *testing.T) {
	root := newRootCmd()

	var violations []string

	for _, cmd := range collectAllCommands(root) {
		short := cmd.Short
		if short == "" {
			continue
		}

		runes := []rune(short)
		if !unicode.IsUpper(runes[0]) {
			violations = append(violations, fmt.Sprintf("%s: starts lowercase: %q", cmd.CommandPath(), short))
		}

		if strings.HasSuffix(short, ".") {
			violations = append(violations, fmt.Sprintf("%s: ends with period: %q", cmd.CommandPath(), short))
		}
	}

	if len(violations) > 0 {
		t.Errorf("Short description style violations:\n  %s\n\nShort must start uppercase and not end with a period.",
			strings.Join(violations, "\n  "))
	}
}

// TestDataCommandsSupportJSON maintains a registry of data-producing commands
// and their --json support status. Any new data command must be explicitly
// registered in either jsonSupported or jsonDeferred, forcing a conscious
// decision about machine-readable output.
func TestDataCommandsSupportJSON(t *testing.T) {
	// Commands that currently support --json output.
	jsonSupported := map[string]bool{
		"mush habitat list":  true,
		"mush history list":  true,
		"mush config list":   true,
		"mush auth status":   true,
		"mush worker status": true,
		"mush version":       true,
	}

	// Commands where --json support is intentionally deferred.
	jsonDeferred := map[string]bool{
		"mush bundle list":  true,
		"mush bundle info":  true,
		"mush doctor":       true,
		"mush config get":   true,
		"mush history view": true,
	}

	// Data verbs that produce output suitable for machine consumption.
	dataVerbs := map[string]bool{
		"list":   true,
		"info":   true,
		"status": true,
		"view":   true,
		"get":    true,
	}

	root := newRootCmd()

	var unregistered []string

	for _, cmd := range collectAllCommands(root) {
		if !cmd.Runnable() {
			continue
		}

		// Extract the verb (last segment of the command path).
		parts := strings.Fields(cmd.CommandPath())
		verb := parts[len(parts)-1]

		if !dataVerbs[verb] {
			continue
		}

		// "version" is a special case — it's a root data command.
		path := cmd.CommandPath()

		if jsonSupported[path] || jsonDeferred[path] {
			continue
		}

		unregistered = append(unregistered, path)
	}

	if len(unregistered) > 0 {
		t.Errorf("data commands not registered for --json support:\n  %s\n\nAdd each command to jsonSupported or jsonDeferred in this test.",
			strings.Join(unregistered, "\n  "))
	}
}

// TestNoShortFlagCollisions checks that no two flags within the same command
// share the same single-letter shorthand.
func TestNoShortFlagCollisions(t *testing.T) {
	root := newRootCmd()

	var collisions []string

	for _, cmd := range collectAllCommands(root) {
		seen := map[string]string{} // shorthand → flag name

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Shorthand == "" {
				return
			}

			if existing, ok := seen[f.Shorthand]; ok {
				collisions = append(collisions,
					fmt.Sprintf("%s: -%s claimed by both --%s and --%s",
						cmd.CommandPath(), f.Shorthand, existing, f.Name))
			}

			seen[f.Shorthand] = f.Name
		})
	}

	if len(collisions) > 0 {
		t.Errorf("short flag collisions:\n  %s",
			strings.Join(collisions, "\n  "))
	}
}

// TestFlagNamesAreKebabCase checks that all flag names follow kebab-case
// naming convention (lowercase letters, digits, and hyphens only).
func TestFlagNamesAreKebabCase(t *testing.T) {
	kebab := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

	root := newRootCmd()

	var violations []string

	for _, cmd := range collectAllCommands(root) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if !kebab.MatchString(f.Name) {
				violations = append(violations,
					fmt.Sprintf("%s: --%s", cmd.CommandPath(), f.Name))
			}
		})
	}

	if len(violations) > 0 {
		t.Errorf("flag names not in kebab-case:\n  %s\n\nUse --kebab-case for all flag names.",
			strings.Join(violations, "\n  "))
	}
}
