package bundle

import "testing"

func TestParseRef(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    Ref
		wantErr bool
	}{
		{
			name: "namespace/slug only",
			arg:  "acme/my-bundle",
			want: Ref{Namespace: "acme", Slug: "my-bundle", Version: ""},
		},
		{
			name: "namespace/slug with version",
			arg:  "acme/my-bundle:0.1.0",
			want: Ref{Namespace: "acme", Slug: "my-bundle", Version: "0.1.0"},
		},
		{
			name: "with whitespace",
			arg:  "  acme/my-bundle  ",
			want: Ref{Namespace: "acme", Slug: "my-bundle", Version: ""},
		},
		{
			name:    "empty string",
			arg:     "",
			wantErr: true,
		},
		{
			name:    "only whitespace",
			arg:     "   ",
			wantErr: true,
		},
		{
			name:    "empty version after colon",
			arg:     "acme/my-bundle:",
			wantErr: true,
		},
		{
			name: "version with dots",
			arg:  "acme/my-bundle:1.2.3",
			want: Ref{Namespace: "acme", Slug: "my-bundle", Version: "1.2.3"},
		},
		{
			name: "version with hash",
			arg:  "acme/my-bundle:abc123",
			want: Ref{Namespace: "acme", Slug: "my-bundle", Version: "abc123"},
		},
		{
			name:    "bare slug without namespace",
			arg:     "my-bundle",
			wantErr: true,
		},
		{
			name:    "bare slug with version without namespace",
			arg:     "my-bundle:1.0.0",
			wantErr: true,
		},
		{
			name:    "empty namespace",
			arg:     "/my-bundle",
			wantErr: true,
		},
		{
			name:    "empty slug",
			arg:     "acme/",
			wantErr: true,
		},
		{
			name:    "empty slug with version",
			arg:     "acme/:1.0.0",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRef(tc.arg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q) expected error, got %+v", tc.arg, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRef(%q) unexpected error: %v", tc.arg, err)
			}

			if got != tc.want {
				t.Fatalf("ParseRef(%q) = %+v, want %+v", tc.arg, got, tc.want)
			}
		})
	}
}

func TestRefString(t *testing.T) {
	tests := []struct {
		ref  Ref
		want string
	}{
		{Ref{Namespace: "acme", Slug: "my-bundle"}, "acme/my-bundle"},
		{Ref{Namespace: "acme", Slug: "my-bundle", Version: "0.1.0"}, "acme/my-bundle:0.1.0"},
	}

	for _, tc := range tests {
		if got := tc.ref.String(); got != tc.want {
			t.Fatalf("Ref.String() = %q, want %q", got, tc.want)
		}
	}
}
