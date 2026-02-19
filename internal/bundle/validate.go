package bundle

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateLogicalPath checks that a logical path is safe and does not attempt
// path traversal. It rejects absolute paths, ".." components, null bytes,
// and paths that resolve outside the target directory.
func ValidateLogicalPath(logicalPath string) error {
	if logicalPath == "" {
		return fmt.Errorf("logical path cannot be empty")
	}

	// Reject null bytes.
	if strings.ContainsRune(logicalPath, 0) {
		return fmt.Errorf("logical path contains null byte: %q", logicalPath)
	}

	// Reject absolute paths.
	if filepath.IsAbs(logicalPath) {
		return fmt.Errorf("logical path must be relative: %s", logicalPath)
	}

	// Reject leading slash (platform-independent).
	if strings.HasPrefix(logicalPath, "/") || strings.HasPrefix(logicalPath, "\\") {
		return fmt.Errorf("logical path must not start with a separator: %s", logicalPath)
	}

	// Clean and check for ".." traversal.
	cleaned := filepath.Clean(logicalPath)

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("logical path must not escape target directory: %s", logicalPath)
	}

	// Also reject ".." components within the path.
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("logical path contains '..' component: %s", logicalPath)
		}
	}

	return nil
}
