package harness

import "github.com/hinshun/vt10x"

// scrollbackLine stores a snapshot of one terminal row's glyphs.
type scrollbackLine struct {
	cells []vt10x.Glyph
}

// scrollbackBuffer is a fixed-capacity ring buffer of terminal lines.
type scrollbackBuffer struct {
	lines    []scrollbackLine
	capacity int
	head     int // next write position
	count    int
}

const defaultScrollbackCapacity = 1000

// newScrollbackBuffer creates a ring buffer with the given line capacity.
func newScrollbackBuffer(capacity int) *scrollbackBuffer {
	if capacity <= 0 {
		capacity = defaultScrollbackCapacity
	}

	return &scrollbackBuffer{
		lines:    make([]scrollbackLine, capacity),
		capacity: capacity,
	}
}

// Push appends a row snapshot to the ring buffer.
func (b *scrollbackBuffer) Push(cells []vt10x.Glyph) {
	cp := make([]vt10x.Glyph, len(cells))
	copy(cp, cells)

	b.lines[b.head] = scrollbackLine{cells: cp}
	b.head = (b.head + 1) % b.capacity

	if b.count < b.capacity {
		b.count++
	}
}

// Line returns the line at offset from the most recent (0 = newest).
// Returns nil if offset is out of range.
func (b *scrollbackBuffer) Line(offset int) []vt10x.Glyph {
	if offset < 0 || offset >= b.count {
		return nil
	}

	// head-1 is the most recent; head-1-offset is the requested line
	idx := (b.head - 1 - offset + b.capacity) % b.capacity

	return b.lines[idx].cells
}

// Len returns the number of lines stored.
func (b *scrollbackBuffer) Len() int {
	return b.count
}
