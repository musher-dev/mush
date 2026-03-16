// Package testutil provides testing utilities for Mush CLI.
package testutil

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/musher-dev/mush/internal/safeio"
)

// update is a flag to update golden files instead of comparing.
// Usage: go test ./path/to/pkg -args -update
// Or:    UPDATE_GOLDEN=1 go test ./...
var update = flag.Bool("update", false, "update golden files")

// shouldUpdate returns true if golden files should be updated.
func shouldUpdate() bool {
	return *update || os.Getenv("UPDATE_GOLDEN") != ""
}

// AssertGolden compares got against a golden file.
// If the -update flag is set, it writes got to the golden file instead.
// The golden file path is relative to the testdata directory.
func AssertGolden(t *testing.T, got, goldenFile string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", goldenFile)

	if shouldUpdate() {
		// Ensure testdata directory exists
		if err := safeio.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("failed to create testdata directory: %v", err)
		}

		if err := safeio.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("failed to update golden file %s: %v", goldenPath, err)
		}

		t.Logf("updated golden file: %s", goldenPath)

		return
	}

	want, err := safeio.ReadFile(goldenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatalf("golden file %s does not exist; run with UPDATE_GOLDEN=1 to create it", goldenPath)
		}

		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	if got != string(want) {
		t.Errorf("output mismatch for %s\n\ngot:\n%s\n\nwant:\n%s\n\nrun with UPDATE_GOLDEN=1 to refresh golden files", goldenPath, got, string(want))
	}
}

// AssertGoldenBytes compares got bytes against a golden file.
func AssertGoldenBytes(t *testing.T, got []byte, goldenFile string) {
	t.Helper()
	AssertGolden(t, string(got), goldenFile)
}

// GoldenPath returns the full path to a golden file in testdata.
func GoldenPath(filename string) string {
	return filepath.Join("testdata", filename)
}

// ReadGolden reads a golden file and returns its contents.
// Returns empty string if the file doesn't exist.
func ReadGolden(t *testing.T, goldenFile string) string {
	t.Helper()

	goldenPath := filepath.Join("testdata", goldenFile)

	data, err := safeio.ReadFile(goldenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}

		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	return string(data)
}
