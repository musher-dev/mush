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

// Line returns the line at index from the oldest retained entry (0 = oldest).
// Returns nil if index is out of range.
func (b *scrollbackBuffer) Line(index int) []vt10x.Glyph {
	if index < 0 || index >= b.count {
		return nil
	}

	oldest := (b.head - b.count + b.capacity) % b.capacity
	idx := (oldest + index) % b.capacity

	return b.lines[idx].cells
}

// Len returns the number of lines stored.
func (b *scrollbackBuffer) Len() int {
	return b.count
}

// Clear drops all retained lines without reallocating the buffer.
func (b *scrollbackBuffer) Clear() {
	b.head = 0
	b.count = 0
}
