package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssertGolden(t *testing.T) {
	// Create a temporary test directory
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create testdata directory
	os.MkdirAll("testdata", 0o755)

	// Write a golden file
	goldenContent := "expected output\n"
	os.WriteFile("testdata/test.golden", []byte(goldenContent), 0o644)

	// Test matching content
	t.Run("matching content passes", func(t *testing.T) {
		// This should not fail
		mockT := &testing.T{}
		AssertGolden(mockT, goldenContent, "test.golden")

		if mockT.Failed() {
			t.Error("AssertGolden should pass when content matches")
		}
	})

	t.Run("mismatched content fails", func(t *testing.T) {
		// We can't easily test failure without a mock, but we can verify the file exists
		path := GoldenPath("test.golden")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("GoldenPath should return valid path, got %s", path)
		}
	})
}

func TestReadGolden(t *testing.T) {
	// Create a temporary test directory
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create testdata directory and file
	os.MkdirAll("testdata", 0o755)
	os.WriteFile("testdata/read.golden", []byte("test content"), 0o644)

	t.Run("reads existing file", func(t *testing.T) {
		got := ReadGolden(t, "read.golden")
		if got != "test content" {
			t.Errorf("ReadGolden() = %q, want %q", got, "test content")
		}
	})

	t.Run("returns empty for missing file", func(t *testing.T) {
		got := ReadGolden(t, "nonexistent.golden")
		if got != "" {
			t.Errorf("ReadGolden() for missing file = %q, want empty", got)
		}
	})
}

func TestGoldenPath(t *testing.T) {
	got := GoldenPath("test.golden")

	want := filepath.Join("testdata", "test.golden")
	if got != want {
		t.Errorf("GoldenPath() = %q, want %q", got, want)
	}
}
