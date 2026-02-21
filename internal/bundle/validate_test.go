package bundle

import (
	"strings"
	"testing"
)

func TestValidateLogicalPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name: "simple relative path",
			path: "agents/researcher.md",
		},
		{
			name: "deeply nested",
			path: "skills/web-search/SKILL.md",
		},
		{
			name: "single file",
			path: "config.json",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "absolute path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "dot-dot traversal",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "dot-dot in middle",
			path:    "agents/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "just dot-dot",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "null byte",
			path:    "agents/\x00evil.md",
			wantErr: true,
		},
		{
			name:    "leading backslash",
			path:    "\\etc\\passwd",
			wantErr: true,
		},
		{
			name: "dot in filename",
			path: "agents/.hidden",
		},
		{
			name: "current dir prefix",
			path: "agents/./test.md",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLogicalPath(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateLogicalPath(%q) expected error, got nil", tc.path)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateLogicalPath(%q) unexpected error: %v", tc.path, err)
			}
		})
	}
}

func TestValidateSkillFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
		errHint string
	}{
		{
			name: "valid frontmatter",
			data: "---\nname: test-skill\ndescription: \"A test skill\"\n---\n# Skill content\n",
		},
		{
			name: "no frontmatter",
			data: "# Just a markdown file\nNo frontmatter here.\n",
		},
		{
			name: "empty file",
			data: "",
		},
		{
			name:    "invalid YAML - unquoted colon",
			data:    "---\nname: test\ndescription: something: broken: value\n---\n# Skill\n",
			wantErr: true,
			errHint: "colons are quoted",
		},
		{
			name:    "invalid YAML - bad indentation",
			data:    "---\nname: test\n  bad:\n indent\n---\n# Skill\n",
			wantErr: true,
		},
		{
			name: "valid frontmatter with quoted colons",
			data: "---\nname: test\ndescription: \"contains: colons: here\"\n---\n# Skill\n",
		},
		{
			name: "frontmatter with no closing delimiter",
			data: "---\nname: test\ndescription: open ended\n",
		},
		{
			name: "multiline frontmatter",
			data: "---\nname: test\ndescription: |\n  A multiline\n  description value\ntags:\n  - one\n  - two\n---\n# Content\n",
		},
		{
			name: "opening delimiter not on its own line",
			data: "---not-frontmatter\nname: test\n---\n",
		},
		{
			name: "valid frontmatter with CRLF line endings",
			data: "---\r\nname: test\r\ndescription: \"a value\"\r\n---\r\n# Content\r\n",
		},
		{
			name:    "invalid YAML with CRLF",
			data:    "---\r\nname: test\r\ndescription: something: broken\r\n---\r\n",
			wantErr: true,
			errHint: "colons are quoted",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSkillFrontmatter([]byte(tc.data))
			if tc.wantErr {
				if err == nil {
					t.Fatal("ValidateSkillFrontmatter() expected error, got nil")
				}

				if tc.errHint != "" && !strings.Contains(err.Error(), tc.errHint) {
					t.Fatalf("error should contain %q, got: %v", tc.errHint, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateSkillFrontmatter() unexpected error: %v", err)
			}
		})
	}
}
