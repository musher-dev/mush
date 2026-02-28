package render

import "testing"

func TestVisibleLength(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "empty string", input: "", want: 0},
		{name: "plain ASCII", input: "hello", want: 5},
		{name: "SGR codes", input: "\x1b[31mhello\x1b[0m", want: 5},
		{name: "CJK characters", input: "你好", want: 4},
		{name: "mixed ASCII and CJK", input: "hi你好", want: 6},
		{name: "colored CJK", input: "\x1b[31m你好\x1b[0m", want: 4},
		{name: "OSC sequence", input: "\x1b]0;title\x07hello", want: 5},
		{name: "only escapes", input: "\x1b[31m\x1b[0m", want: 0},
		{name: "adjacent escapes", input: "\x1b[1m\x1b[31mhi\x1b[0m", want: 2},
		{name: "single emoji", input: "🌍", want: 2},
		{name: "combining character", input: "e\u0301", want: 1},
		{name: "nF escape", input: "\x1b(Bhello", want: 5},
		{name: "tab character", input: "a\tb", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisibleLength(tt.input); got != tt.want {
				t.Fatalf("VisibleLength(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestPadRightVisible(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{
			name:  "ASCII padding",
			input: "hi",
			width: 5,
			want:  "hi   ",
		},
		{
			name:  "CJK padding",
			input: "你好",
			width: 6,
			want:  "你好  ",
		},
		{
			name:  "no padding needed",
			input: "hello",
			width: 3,
			want:  "hello",
		},
		{
			name:  "ANSI string padded to exact visible width",
			input: "\x1b[31mhi\x1b[0m",
			width: 2,
			want:  "\x1b[31mhi\x1b[0m",
		},
		{
			name:  "width zero",
			input: "hello",
			width: 0,
			want:  "hello",
		},
		{
			name:  "negative width",
			input: "hello",
			width: -1,
			want:  "hello",
		},
		{
			name:  "wide characters exceed width",
			input: "你好",
			width: 3,
			want:  "你好",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PadRightVisible(tt.input, tt.width)
			if got != tt.want {
				t.Fatalf("PadRightVisible(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
		})
	}
}
