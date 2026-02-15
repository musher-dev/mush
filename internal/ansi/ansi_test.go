package ansi

import "testing"

func TestStrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text",
			in:   "hello world",
			want: "hello world",
		},
		{
			name: "single color sequence",
			in:   "\x1b[31mred\x1b[0m text",
			want: "red text",
		},
		{
			name: "multiple sequences",
			in:   "a\x1b[1mb\x1b[0mc\x1b[32md\x1b[0m",
			want: "abcd",
		},
		{
			name: "unterminated sequence retained",
			in:   "hello \x1b[31",
			want: "hello \x1b[31",
		},
		{
			name: "unicode around ansi",
			in:   "✓ \x1b[36mblue\x1b[0m 你好",
			want: "✓ blue 你好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Strip(tt.in); got != tt.want {
				t.Fatalf("Strip() = %q, want %q", got, tt.want)
			}
		})
	}
}
