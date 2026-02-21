package bundle

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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

// ValidateSkillFrontmatter extracts YAML frontmatter (between --- delimiters)
// from a SKILL.md file and validates it parses as YAML. Returns nil if valid
// or if no frontmatter is present.
func ValidateSkillFrontmatter(data []byte) error {
	frontmatter := extractFrontmatter(data)
	if frontmatter == nil {
		return nil
	}

	var doc any
	if err := yaml.Unmarshal(frontmatter, &doc); err != nil {
		return fmt.Errorf("invalid YAML frontmatter: %w (hint: ensure values containing colons are quoted)", err)
	}

	return nil
}

// extractFrontmatter returns the YAML frontmatter bytes between --- delimiters,
// or nil if no frontmatter is found. Handles both \n and \r\n line endings and
// requires the opening/closing delimiters to be on their own lines.
func extractFrontmatter(data []byte) []byte {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return nil
	}

	// Find end of the first line.
	firstNL := bytes.IndexByte(trimmed, '\n')
	if firstNL < 0 {
		return nil
	}

	// Opening delimiter must be exactly "---" (with optional trailing \r/spaces).
	firstLine := bytes.Trim(trimmed[:firstNL], " \t\r")
	if !bytes.Equal(firstLine, []byte("---")) {
		return nil
	}

	contentStart := firstNL + 1
	if contentStart >= len(trimmed) {
		return nil
	}

	// Scan subsequent lines for a closing delimiter.
	i := contentStart

	for i < len(trimmed) {
		j := bytes.IndexByte(trimmed[i:], '\n')

		var line []byte

		var next int

		if j < 0 {
			line = trimmed[i:]
			next = len(trimmed)
		} else {
			line = trimmed[i : i+j]
			next = i + j + 1
		}

		if bytes.Equal(bytes.Trim(line, " \t\r"), []byte("---")) {
			return trimmed[contentStart:i]
		}

		i = next
	}

	return nil
}
