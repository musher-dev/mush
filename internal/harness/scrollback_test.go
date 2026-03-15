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

	if got := glyphsToString(buf.Line(0)); got != "line0" {
		t.Fatalf("Line(0) = %q, want %q", got, "line0")
	}

	if got := glyphsToString(buf.Line(1)); got != "line1" {
		t.Fatalf("Line(1) = %q, want %q", got, "line1")
	}

	if got := glyphsToString(buf.Line(2)); got != "line2" {
		t.Fatalf("Line(2) = %q, want %q", got, "line2")
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

	if got := glyphsToString(buf.Line(0)); got != "c" {
		t.Fatalf("Line(0) = %q, want %q", got, "c")
	}

	if got := glyphsToString(buf.Line(1)); got != "d" {
		t.Fatalf("Line(1) = %q, want %q", got, "d")
	}

	if got := glyphsToString(buf.Line(2)); got != "e" {
		t.Fatalf("Line(2) = %q, want %q", got, "e")
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

func TestViewportTop_AnchoredDuringNewOutput(t *testing.T) {
	buf := newScrollbackBuffer(100)

	// Push some initial lines.
	buf.Push(makeGlyphs("line0"))
	buf.Push(makeGlyphs("line1"))
	buf.Push(makeGlyphs("line2"))

	viewportTop := 1
	target := glyphsToString(buf.Line(viewportTop))

	// New output arrives: 3 more lines scroll off into the buffer.
	buf.Push(makeGlyphs("line3"))
	buf.Push(makeGlyphs("line4"))
	buf.Push(makeGlyphs("line5"))

	scrolledOff := 3

	// Anchoring retained history rows should keep the same absolute index.
	if scrolledOff > 0 && viewportTop >= buf.Len()-scrolledOff {
		viewportTop += scrolledOff
		if viewportTop > buf.Len()-1 {
			viewportTop = buf.Len() - 1
		}
	}

	got := glyphsToString(buf.Line(viewportTop))
	if got != target {
		t.Fatalf("after anchoring, Line(%d) = %q, want %q", viewportTop, got, target)
	}
}

func TestViewportTop_ClampedOnWrapAround(t *testing.T) {
	buf := newScrollbackBuffer(5)

	// Fill buffer to capacity.
	for range 5 {
		buf.Push(makeGlyphs("old"))
	}

	viewportTop := 4 // near the newest retained row

	// Push enough to wrap the ring buffer.
	for range 10 {
		buf.Push(makeGlyphs("new"))
	}

	scrolledOff := 10

	if scrolledOff > 0 && viewportTop >= buf.Len()-scrolledOff {
		viewportTop += scrolledOff
		if viewportTop > buf.Len()-1 {
			viewportTop = buf.Len() - 1
		}
	}

	if viewportTop != 4 {
		t.Fatalf("expected viewportTop to stay anchored at 4, got %d", viewportTop)
	}

	if line := buf.Line(viewportTop); line == nil {
		t.Fatalf("expected retained line for Line(%d), got nil", viewportTop)
	}
}

func TestScrollbackBuffer_DefaultCapacity(t *testing.T) {
	buf := newScrollbackBuffer(0)

	if buf.capacity != defaultScrollbackCapacity {
		t.Fatalf("expected default capacity %d, got %d", defaultScrollbackCapacity, buf.capacity)
	}
}

func TestScrollbackBuffer_Clear(t *testing.T) {
	buf := newScrollbackBuffer(5)
	buf.Push(makeGlyphs("line0"))
	buf.Push(makeGlyphs("line1"))

	buf.Clear()

	if buf.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", buf.Len())
	}

	if line := buf.Line(0); line != nil {
		t.Fatalf("expected nil after Clear(), got %v", line)
	}
}
