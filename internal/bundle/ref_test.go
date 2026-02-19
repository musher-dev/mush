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
			name: "slug only",
			arg:  "my-bundle",
			want: Ref{Slug: "my-bundle", Version: ""},
		},
		{
			name: "slug with version",
			arg:  "my-bundle:0.1.0",
			want: Ref{Slug: "my-bundle", Version: "0.1.0"},
		},
		{
			name: "slug with whitespace",
			arg:  "  my-bundle  ",
			want: Ref{Slug: "my-bundle", Version: ""},
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
			arg:     "my-bundle:",
			wantErr: true,
		},
		{
			name: "version with dots",
			arg:  "my-bundle:1.2.3",
			want: Ref{Slug: "my-bundle", Version: "1.2.3"},
		},
		{
			name: "version with hash",
			arg:  "my-bundle:abc123",
			want: Ref{Slug: "my-bundle", Version: "abc123"},
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
		{Ref{Slug: "my-bundle"}, "my-bundle"},
		{Ref{Slug: "my-bundle", Version: "0.1.0"}, "my-bundle:0.1.0"},
	}

	for _, tc := range tests {
		if got := tc.ref.String(); got != tc.want {
			t.Fatalf("Ref.String() = %q, want %q", got, tc.want)
		}
	}
}
