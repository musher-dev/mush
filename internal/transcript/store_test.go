package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAppendReadAndList(t *testing.T) {
	tmp := t.TempDir()

	s, err := NewStore(StoreOptions{
		SessionID: "s-1",
		Dir:       tmp,
		MaxLines:  3,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	err = s.Append("pty", []byte("a\nb\n"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	err = s.Append("pty", []byte("c\nd\n"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	lines := s.SnapshotLines()
	if len(lines) != 3 {
		t.Fatalf("SnapshotLines len = %d, want 3", len(lines))
	}

	if lines[0] != "b" || lines[1] != "c" || lines[2] != "d" {
		t.Fatalf("SnapshotLines = %#v, want [b c d]", lines)
	}

	// Read live events BEFORE close (live file is removed after close).
	live, nextOffset, err := ReadLiveEventsFrom(tmp, "s-1", 0)
	if err != nil {
		t.Fatalf("ReadLiveEventsFrom() error = %v", err)
	}

	if len(live) != 2 {
		t.Fatalf("ReadLiveEventsFrom len = %d, want 2", len(live))
	}

	if nextOffset <= 0 {
		t.Fatalf("ReadLiveEventsFrom nextOffset = %d, want > 0", nextOffset)
	}

	err = s.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// After close, live file is removed — ReadLiveEventsFrom returns empty.
	liveAfter, _, err := ReadLiveEventsFrom(tmp, "s-1", 0)
	if err != nil {
		t.Fatalf("ReadLiveEventsFrom after close error = %v", err)
	}

	if len(liveAfter) != 0 {
		t.Fatalf("ReadLiveEventsFrom after close len = %d, want 0", len(liveAfter))
	}

	evs, err := ReadEvents(tmp, "s-1")
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}

	if len(evs) != 2 {
		t.Fatalf("ReadEvents len = %d, want 2", len(evs))
	}

	list, err := ListSessions(tmp)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}

	if len(list) != 1 || list[0].SessionID != "s-1" {
		t.Fatalf("ListSessions() = %#v", list)
	}
}

func TestPruneOlderThan(t *testing.T) {
	tmp := t.TempDir()

	oldStore, err := NewStore(StoreOptions{SessionID: "old", Dir: tmp})
	if err != nil {
		t.Fatalf("NewStore old error = %v", err)
	}

	err = oldStore.Append("pty", []byte("old\n"))
	if err != nil {
		t.Fatalf("Append old error = %v", err)
	}

	err = oldStore.Close()
	if err != nil {
		t.Fatalf("Close old error = %v", err)
	}

	newStore, err := NewStore(StoreOptions{SessionID: "new", Dir: tmp})
	if err != nil {
		t.Fatalf("NewStore new error = %v", err)
	}

	err = newStore.Append("pty", []byte("new\n"))
	if err != nil {
		t.Fatalf("Append new error = %v", err)
	}

	err = newStore.Close()
	if err != nil {
		t.Fatalf("Close new error = %v", err)
	}

	// make "old" metadata old by replacing startedAt via prune cutoff in future.
	cutoff := time.Now().Add(1 * time.Hour)

	removed, err := PruneOlderThan(tmp, cutoff)
	if err != nil {
		t.Fatalf("PruneOlderThan error = %v", err)
	}

	if removed != 2 {
		t.Fatalf("PruneOlderThan removed = %d, want 2", removed)
	}

	// best effort sanity: folder should be gone — no events returned.
	evs, err := ReadEvents(tmp, "old")
	if err != nil {
		t.Fatalf("ReadEvents old after prune error = %v", err)
	}

	if len(evs) != 0 {
		t.Fatalf("ReadEvents old after prune len = %d, want 0", len(evs))
	}
}

func TestReadLiveEventsFromOffset(t *testing.T) {
	tmp := t.TempDir()

	s, err := NewStore(StoreOptions{SessionID: "offset", Dir: tmp})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	defer func() {
		_ = s.Close()
	}()

	err = s.Append("pty", []byte("one\n"))
	if err != nil {
		t.Fatalf("Append one error = %v", err)
	}

	first, offset, err := ReadLiveEventsFrom(tmp, "offset", 0)
	if err != nil {
		t.Fatalf("ReadLiveEventsFrom first error = %v", err)
	}

	if len(first) != 1 {
		t.Fatalf("first len = %d, want 1", len(first))
	}

	err = s.Append("pty", []byte("two\n"))
	if err != nil {
		t.Fatalf("Append two error = %v", err)
	}

	second, nextOffset, err := ReadLiveEventsFrom(tmp, "offset", offset)
	if err != nil {
		t.Fatalf("ReadLiveEventsFrom second error = %v", err)
	}

	if len(second) != 1 {
		t.Fatalf("second len = %d, want 1", len(second))
	}

	if second[0].Text != "two\n" {
		t.Fatalf("second[0].Text = %q, want %q", second[0].Text, "two\n")
	}

	if nextOffset <= offset {
		t.Fatalf("nextOffset = %d, want > %d", nextOffset, offset)
	}
}

func TestValidateSessionIDRejectsSeparators(t *testing.T) {
	tests := []string{
		"../bad",
		"bad/child",
		`bad\child`,
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			if err := validateSessionID(tc); err == nil {
				t.Fatalf("validateSessionID(%q) expected error", tc)
			}
		})
	}
}

func TestReadLiveEventsFromMissingFileReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()

	dir := filepath.Join(tmp, "missing")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	events, offset, err := ReadLiveEventsFrom(tmp, "missing", 0)
	if err != nil {
		t.Fatalf("ReadLiveEventsFrom error = %v", err)
	}

	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0", len(events))
	}

	if offset != 0 {
		t.Fatalf("offset = %d, want 0", offset)
	}
}

func TestReadEventsFromCrashedSession(t *testing.T) {
	tmp := t.TempDir()

	s, err := NewStore(StoreOptions{SessionID: "crashed", Dir: tmp})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	err = s.Append("pty", []byte("hello\n"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	err = s.Append("pty", []byte("world\n"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Simulate crash: flush live writer but do NOT call Close().
	// This leaves events.live.jsonl but no events.jsonl.gz.
	s.mu.Lock()
	_ = s.liveBW.Flush()
	_ = s.liveFile.Close()
	s.mu.Unlock()

	// ReadEvents should fall back to the live file.
	evs, err := ReadEvents(tmp, "crashed")
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}

	if len(evs) != 2 {
		t.Fatalf("ReadEvents len = %d, want 2", len(evs))
	}

	if evs[0].Text != "hello\n" {
		t.Fatalf("evs[0].Text = %q, want %q", evs[0].Text, "hello\n")
	}
}

func TestCloseCompressesAndRemovesLiveFile(t *testing.T) {
	tmp := t.TempDir()

	s, err := NewStore(StoreOptions{SessionID: "compress", Dir: tmp})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	err = s.Append("pty", []byte("data\n"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	err = s.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	sessionDir := filepath.Join(tmp, "compress")

	// events.jsonl.gz should exist.
	gzPath := filepath.Join(sessionDir, "events.jsonl.gz")
	if _, statErr := os.Stat(gzPath); statErr != nil {
		t.Fatalf("events.jsonl.gz missing after close: %v", statErr)
	}

	// events.live.jsonl should NOT exist.
	livePath := filepath.Join(sessionDir, "events.live.jsonl")
	if _, statErr := os.Stat(livePath); !os.IsNotExist(statErr) {
		t.Fatalf("events.live.jsonl still exists after close (err=%v)", statErr)
	}

	// ReadEvents should work from the compressed file.
	evs, err := ReadEvents(tmp, "compress")
	if err != nil {
		t.Fatalf("ReadEvents() error = %v", err)
	}

	if len(evs) != 1 {
		t.Fatalf("ReadEvents len = %d, want 1", len(evs))
	}

	if evs[0].Text != "data\n" {
		t.Fatalf("evs[0].Text = %q, want %q", evs[0].Text, "data\n")
	}
}
