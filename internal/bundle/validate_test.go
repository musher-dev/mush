package bundle

import "testing"

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
