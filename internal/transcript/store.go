package transcript

import (
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultLines          = 10000
	defaultRetentionHours = 24 * 30
	eventsFileName        = "events.jsonl.gz"
	eventsLiveFileName    = "events.live.jsonl"
	metaFileName          = "meta.json"
)

// Event is a single transcript record.
type Event struct {
	SessionID string    `json:"sessionId"`
	Seq       uint64    `json:"seq"`
	TS        time.Time `json:"ts"`
	Stream    string    `json:"stream"`
	RawBase64 string    `json:"rawBase64"`
	Text      string    `json:"text,omitempty"`
}

// Meta stores session metadata for discovery and pruning.
type Meta struct {
	SessionID string     `json:"sessionId"`
	StartedAt time.Time  `json:"startedAt"`
	ClosedAt  *time.Time `json:"closedAt,omitempty"`
}

// StoreOptions controls transcript behavior.
type StoreOptions struct {
	SessionID string
	Dir       string
	MaxLines  int
}

// Store writes transcript events to compressed and live JSONL files and keeps an in-memory ring.
type Store struct {
	mu sync.Mutex

	sessionID string
	dir       string
	maxLines  int
	seq       uint64
	startedAt time.Time

	file     *os.File
	gz       *gzip.Writer
	bw       *bufio.Writer
	liveFile *os.File
	liveBW   *bufio.Writer

	lines       []string
	lineStart   int
	lineCount   int
	partialLine string
	closed      bool
}

// NewStore creates a transcript store for one session.
func NewStore(opts StoreOptions) (*Store, error) {
	if opts.SessionID == "" {
		return nil, errors.New("session id is required")
	}

	if err := validateSessionID(opts.SessionID); err != nil {
		return nil, err
	}

	dir := opts.Dir
	if dir == "" {
		var err error

		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}

	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = defaultLines
	}

	sessionDir := filepath.Join(dir, opts.SessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("create transcript dir: %w", err)
	}

	f, err := os.OpenFile(filepath.Join(sessionDir, eventsFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // sessionDir/sessionID are validated and controlled
	if err != nil {
		return nil, fmt.Errorf("open transcript events: %w", err)
	}

	liveFile, err := os.OpenFile(filepath.Join(sessionDir, eventsLiveFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // sessionDir/sessionID are validated and controlled
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open live transcript events: %w", err)
	}

	gz := gzip.NewWriter(f)
	bw := bufio.NewWriterSize(gz, 64*1024)
	liveBW := bufio.NewWriterSize(liveFile, 64*1024)

	s := &Store{
		sessionID: opts.SessionID,
		dir:       sessionDir,
		maxLines:  maxLines,
		startedAt: time.Now().UTC(),
		file:      f,
		gz:        gz,
		bw:        bw,
		liveFile:  liveFile,
		liveBW:    liveBW,
		lines:     make([]string, maxLines),
	}

	if err := s.writeMeta(&Meta{
		SessionID: opts.SessionID,
		StartedAt: s.startedAt,
	}); err != nil {
		_ = s.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) writeMeta(meta *Meta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal transcript meta: %w", err)
	}

	if err := os.WriteFile(filepath.Join(s.dir, metaFileName), data, 0o600); err != nil {
		return fmt.Errorf("write transcript meta: %w", err)
	}

	return nil
}

// SessionID returns the store's session id.
func (s *Store) SessionID() string {
	return s.sessionID
}

// Append writes one event and updates in-memory line ring.
func (s *Store) Append(stream string, chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("transcript store is closed")
	}

	text := string(chunk)
	s.seq++
	ev := Event{
		SessionID: s.sessionID,
		Seq:       s.seq,
		TS:        time.Now().UTC(),
		Stream:    stream,
		RawBase64: base64.StdEncoding.EncodeToString(chunk),
		Text:      text,
	}

	line, err := json.Marshal(&ev)
	if err != nil {
		return fmt.Errorf("marshal transcript event: %w", err)
	}

	line = append(line, '\n')
	if _, err := s.bw.Write(line); err != nil {
		return fmt.Errorf("encode transcript event: %w", err)
	}

	if _, err := s.liveBW.Write(line); err != nil {
		return fmt.Errorf("encode live transcript event: %w", err)
	}

	if err := s.liveBW.Flush(); err != nil {
		return fmt.Errorf("flush live transcript event: %w", err)
	}

	s.appendLinesLocked(text)

	return nil
}

func (s *Store) appendLinesLocked(text string) {
	combined := s.partialLine + text

	parts := strings.Split(combined, "\n")
	if len(parts) == 0 {
		return
	}

	for i := 0; i < len(parts)-1; i++ {
		s.pushLineLocked(strings.TrimRight(parts[i], "\r"))
	}

	s.partialLine = parts[len(parts)-1]
}

func (s *Store) pushLineLocked(line string) {
	if s.maxLines <= 0 {
		return
	}

	if s.lineCount < s.maxLines {
		idx := (s.lineStart + s.lineCount) % s.maxLines
		s.lines[idx] = line
		s.lineCount++

		return
	}

	s.lines[s.lineStart] = line
	s.lineStart = (s.lineStart + 1) % s.maxLines
}

// SnapshotLines returns the in-memory ring in chronological order.
func (s *Store) SnapshotLines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, s.lineCount)
	for i := 0; i < s.lineCount; i++ {
		idx := (s.lineStart + i) % s.maxLines
		out = append(out, s.lines[idx])
	}

	if s.partialLine != "" {
		out = append(out, s.partialLine)
	}

	return out
}

// Close flushes and closes the transcript.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	now := time.Now().UTC()

	var errs []error
	if err := s.writeMeta(&Meta{
		SessionID: s.sessionID,
		StartedAt: s.startedAt,
		ClosedAt:  &now,
	}); err != nil {
		errs = append(errs, err)
	}

	if s.bw != nil {
		if err := s.bw.Flush(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.liveBW != nil {
		if err := s.liveBW.Flush(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.gz != nil {
		if err := s.gz.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.file != nil {
		if err := s.file.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.liveFile != nil {
		if err := s.liveFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}

	if sessionID != filepath.Base(sessionID) || strings.Contains(sessionID, "..") || strings.ContainsAny(sessionID, `/\`) {
		return errors.New("invalid session id")
	}

	return nil
}
