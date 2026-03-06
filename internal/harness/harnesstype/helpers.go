//go:build unix

package harnesstype

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"syscall"
)

// SummarizeMCPServers returns a comma-separated list of MCP server names.
func SummarizeMCPServers(names []string) string {
	if len(names) == 0 {
		return "none"
	}

	return strings.Join(names, ", ")
}

// SameStringSlice reports whether two string slices contain the same elements
// (order-independent).
func SameStringSlice(expected, compared []string) bool {
	if len(expected) != len(compared) {
		return false
	}

	aCopy := make([]string, len(expected))
	copy(aCopy, expected)

	bCopy := make([]string, len(compared))
	copy(bCopy, compared)

	sort.Strings(aCopy)
	sort.Strings(bCopy)

	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}

	return true
}

// AnnotateStartPTYError adds context to EPERM errors during PTY start.
func AnnotateStartPTYError(err error, binaryPath string) error {
	if !errors.Is(err, syscall.EPERM) {
		return err
	}

	return fmt.Errorf(
		"%w (EPERM during PTY start for %q; likely session/exec policy issue. Check executable permissions, filesystem noexec, and macOS quarantine attributes)",
		err,
		binaryPath,
	)
}
