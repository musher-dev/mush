package render

import "testing"

func TestVisibleLength(t *testing.T) {
	got := VisibleLength("\x1b[31mhello\x1b[0m")
	if got != 5 {
		t.Fatalf("VisibleLength = %d, want 5", got)
	}
}
