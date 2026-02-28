package ansi

import "testing"

func TestStrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// --- existing cases ---
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
		// --- new ECMA-48 parser cases ---
		{
			name: "CSI with tilde final byte",
			in:   "\x1b[15~hello",
			want: "hello",
		},
		{
			name: "OSC with BEL terminator",
			in:   "\x1b]0;title\x07hello",
			want: "hello",
		},
		{
			name: "OSC with ST terminator",
			in:   "\x1b]0;title\x1b\\hello",
			want: "hello",
		},
		{
			name: "DCS sequence",
			in:   "\x1bP1;2;3\x1b\\hello",
			want: "hello",
		},
		{
			name: "Fe two-byte escape ESC7",
			in:   "\x1b7hello\x1b8",
			want: "hello",
		},
		{
			name: "Fe two-byte escape ESCc hard reset",
			in:   "\x1bchello",
			want: "hello",
		},
		{
			name: "mixed CSI and OSC",
			in:   "\x1b]0;t\x07\x1b[31mred\x1b[0m",
			want: "red",
		},
		{
			name: "unterminated OSC retained",
			in:   "hello\x1b]0;title",
			want: "hello\x1b]0;title",
		},
		{
			name: "PM sequence",
			in:   "\x1b^private\x1b\\hello",
			want: "hello",
		},
		{
			name: "APC sequence",
			in:   "\x1b_app\x1b\\hello",
			want: "hello",
		},
		// --- nF escape sequences ---
		{
			name: "nF escape: designate G0 charset ESC ( B",
			in:   "\x1b(Bhello",
			want: "hello",
		},
		{
			name: "nF escape: DECALN ESC # 8",
			in:   "\x1b#8hello",
			want: "hello",
		},
		{
			name: "nF escape: multiple intermediate bytes",
			in:   "\x1b$ Bhello",
			want: "hello",
		},
		{
			name: "nF escape: unterminated at end of string",
			in:   "hello\x1b(",
			want: "hello\x1b(",
		},
		// --- SOS (ESC X) ---
		{
			name: "SOS sequence",
			in:   "\x1bXsome data\x1b\\hello",
			want: "hello",
		},
		{
			name: "SOS unterminated",
			in:   "hello\x1bXsome data",
			want: "hello\x1bXsome data",
		},
		// --- additional edge cases ---
		{
			name: "adjacent escape sequences",
			in:   "\x1b[31m\x1b[1mhello\x1b[0m",
			want: "hello",
		},
		{
			name: "only escape sequences",
			in:   "\x1b[31m\x1b[0m",
			want: "",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "emoji passthrough",
			in:   "hello 🌍 world",
			want: "hello 🌍 world",
		},
		{
			name: "combining characters passthrough",
			in:   "e\u0301 cafe\u0301",
			want: "e\u0301 cafe\u0301",
		},
		{
			name: "ZWJ emoji passthrough",
			in:   "👨\u200d👩\u200d👧\u200d👦",
			want: "👨\u200d👩\u200d👧\u200d👦",
		},
		{
			name: "tab character passthrough",
			in:   "a\tb",
			want: "a\tb",
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
