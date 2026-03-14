package harness

import (
	"testing"

	"github.com/hinshun/vt10x"
)

func makeGlyphs(s string) []vt10x.Glyph {
	glyphs := make([]vt10x.Glyph, len(s))
	for i, ch := range s {
		glyphs[i] = vt10x.Glyph{Char: ch}
	}

	return glyphs
}

func glyphsToString(glyphs []vt10x.Glyph) string {
	runes := make([]rune, len(glyphs))
	for i, g := range glyphs {
		runes[i] = g.Char
	}

	return string(runes)
}

func TestScrollbackBuffer_PushAndLine(t *testing.T) {
	buf := newScrollbackBuffer(5)

	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got Len=%d", buf.Len())
	}

	if line := buf.Line(0); line != nil {
		t.Fatalf("expected nil for empty buffer, got %v", line)
	}

	buf.Push(makeGlyphs("line0"))
	buf.Push(makeGlyphs("line1"))
	buf.Push(makeGlyphs("line2"))

	if buf.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", buf.Len())
	}

	// offset 0 = newest
	if got := glyphsToString(buf.Line(0)); got != "line2" {
		t.Fatalf("Line(0) = %q, want %q", got, "line2")
	}

	if got := glyphsToString(buf.Line(1)); got != "line1" {
		t.Fatalf("Line(1) = %q, want %q", got, "line1")
	}

	if got := glyphsToString(buf.Line(2)); got != "line0" {
		t.Fatalf("Line(2) = %q, want %q", got, "line0")
	}

	// out of range
	if line := buf.Line(3); line != nil {
		t.Fatalf("expected nil for out-of-range, got %v", line)
	}

	if line := buf.Line(-1); line != nil {
		t.Fatalf("expected nil for negative offset, got %v", line)
	}
}

func TestScrollbackBuffer_WrapAround(t *testing.T) {
	buf := newScrollbackBuffer(3)

	buf.Push(makeGlyphs("a"))
	buf.Push(makeGlyphs("b"))
	buf.Push(makeGlyphs("c"))
	buf.Push(makeGlyphs("d")) // overwrites "a"
	buf.Push(makeGlyphs("e")) // overwrites "b"

	if buf.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", buf.Len())
	}

	if got := glyphsToString(buf.Line(0)); got != "e" {
		t.Fatalf("Line(0) = %q, want %q", got, "e")
	}

	if got := glyphsToString(buf.Line(1)); got != "d" {
		t.Fatalf("Line(1) = %q, want %q", got, "d")
	}

	if got := glyphsToString(buf.Line(2)); got != "c" {
		t.Fatalf("Line(2) = %q, want %q", got, "c")
	}

	if line := buf.Line(3); line != nil {
		t.Fatalf("expected nil for wrapped-out line, got %v", line)
	}
}

func TestScrollbackBuffer_PushCopiesData(t *testing.T) {
	buf := newScrollbackBuffer(5)
	cells := makeGlyphs("hello")
	buf.Push(cells)

	// mutate original
	cells[0].Char = 'X'

	if got := glyphsToString(buf.Line(0)); got != "hello" {
		t.Fatalf("mutation leaked: Line(0) = %q, want %q", got, "hello")
	}
}

func TestScrollOffset_AnchoredDuringNewOutput(t *testing.T) {
	buf := newScrollbackBuffer(100)

	// Push some initial lines.
	buf.Push(makeGlyphs("line0"))
	buf.Push(makeGlyphs("line1"))
	buf.Push(makeGlyphs("line2"))

	// Simulate user scrolled up 2 lines — viewing line1 and line0.
	scrollOffset := 2
	target := glyphsToString(buf.Line(scrollOffset)) // "line0"

	// New output arrives: 3 more lines scroll off into the buffer.
	buf.Push(makeGlyphs("line3"))
	buf.Push(makeGlyphs("line4"))
	buf.Push(makeGlyphs("line5"))

	scrolledOff := 3

	// Anchor the offset.
	if scrolledOff > 0 && scrollOffset > 0 {
		scrollOffset += scrolledOff
		if scrollOffset > buf.Len() {
			scrollOffset = buf.Len()
		}
	}

	got := glyphsToString(buf.Line(scrollOffset))
	if got != target {
		t.Fatalf("after anchoring, Line(%d) = %q, want %q", scrollOffset, got, target)
	}
}

func TestScrollOffset_ClampedOnWrapAround(t *testing.T) {
	buf := newScrollbackBuffer(5)

	// Fill buffer to capacity.
	for range 5 {
		buf.Push(makeGlyphs("old"))
	}

	scrollOffset := 4 // near the top of history

	// Push enough to wrap the ring buffer.
	for range 10 {
		buf.Push(makeGlyphs("new"))
	}

	scrolledOff := 10

	if scrolledOff > 0 && scrollOffset > 0 {
		scrollOffset += scrolledOff
		if scrollOffset > buf.Len() {
			scrollOffset = buf.Len()
		}
	}

	if scrollOffset != buf.Len() {
		t.Fatalf("expected offset clamped to %d, got %d", buf.Len(), scrollOffset)
	}

	// Len() is one past the last valid index, so Line(Len()) should be nil.
	// This confirms the clamp prevents out-of-bounds reads.
	if line := buf.Line(scrollOffset); line != nil {
		t.Fatalf("expected nil for Line(%d), got non-nil", scrollOffset)
	}
}

func TestScrollOffset_UnchangedWhenLive(t *testing.T) {
	buf := newScrollbackBuffer(100)
	scrollOffset := 0 // live mode — not scrolled

	buf.Push(makeGlyphs("line0"))
	buf.Push(makeGlyphs("line1"))

	scrolledOff := 2

	// Guard should prevent modification when scrollOffset == 0.
	if scrolledOff > 0 && scrollOffset > 0 {
		scrollOffset += scrolledOff
		if scrollOffset > buf.Len() {
			scrollOffset = buf.Len()
		}
	}

	if scrollOffset != 0 {
		t.Fatalf("expected scrollOffset to remain 0 in live mode, got %d", scrollOffset)
	}
}

func TestScrollbackBuffer_DefaultCapacity(t *testing.T) {
	buf := newScrollbackBuffer(0)

	if buf.capacity != defaultScrollbackCapacity {
		t.Fatalf("expected default capacity %d, got %d", defaultScrollbackCapacity, buf.capacity)
	}
}
