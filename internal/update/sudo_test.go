//go:build !windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsElevation_WritableDir(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "mush")

	if NeedsElevation(binPath) {
		t.Error("NeedsElevation returned true for writable directory")
	}
}

func TestNeedsElevation_ReadOnlyDir(t *testing.T) {
	tmp := t.TempDir()
	readOnly := filepath.Join(tmp, "readonly")
	if err := os.MkdirAll(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	// Ensure cleanup can remove the dir
	t.Cleanup(func() { os.Chmod(readOnly, 0o755) })

	binPath := filepath.Join(readOnly, "mush")

	if !NeedsElevation(binPath) {
		t.Error("NeedsElevation returned false for read-only directory")
	}
}
