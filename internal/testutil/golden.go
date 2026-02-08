// Package testutil provides testing utilities for Mush CLI.
package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update is a flag to update golden files instead of comparing.
// Usage: go test ./... -update
var update = flag.Bool("update", false, "update golden files")

// AssertGolden compares got against a golden file.
// If the -update flag is set, it writes got to the golden file instead.
// The golden file path is relative to the testdata directory.
func AssertGolden(t *testing.T, got, goldenFile string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", goldenFile)

	if *update {
		// Ensure testdata directory exists
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("failed to create testdata directory: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("failed to update golden file %s: %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file %s does not exist; run with -update to create it", goldenPath)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	if got != string(want) {
		t.Errorf("output mismatch for %s\n\ngot:\n%s\n\nwant:\n%s\n\nrun with -update to refresh golden files", goldenPath, got, string(want))
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
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}
	return string(data)
}
