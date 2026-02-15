package transcript

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// Session describes one stored transcript session.
type Session struct {
	SessionID string
	Path      string
	StartedAt time.Time
	ClosedAt  *time.Time
}

// ListSessions returns transcript sessions sorted by newest start time first.
func ListSessions(rootDir string) ([]Session, error) {
	if rootDir == "" {
		var err error
		rootDir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sessions := make([]Session, 0, len(entries))
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		dir := filepath.Join(rootDir, ent.Name())
		metaPath := filepath.Join(dir, metaFileName)
		data, err := os.ReadFile(metaPath) //nolint:gosec // controlled directory
		if err != nil {
			continue
		}
		var meta Meta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		sessions = append(sessions, Session{
			SessionID: meta.SessionID,
			Path:      dir,
			StartedAt: meta.StartedAt,
			ClosedAt:  meta.ClosedAt,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.After(sessions[j].StartedAt)
	})
	return sessions, nil
}

// ReadEvents reads all events for a given session.
func ReadEvents(rootDir, sessionID string) (events []Event, err error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if validateErr := validateSessionID(sessionID); validateErr != nil {
		return nil, validateErr
	}
	if rootDir == "" {
		var resolveErr error
		rootDir, resolveErr = DefaultDir()
		if resolveErr != nil {
			return nil, resolveErr
		}
	}

	path := filepath.Join(rootDir, sessionID, eventsFileName)
	f, err := os.Open(path) //nolint:gosec // controlled path
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	sc := bufio.NewScanner(gz)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return events, nil
}

// ReadLiveEventsFrom reads live transcript events from a byte offset in the append-only JSONL file.
func ReadLiveEventsFrom(rootDir, sessionID string, offset int64) (events []Event, nextOffset int64, err error) {
	if sessionID == "" {
		return nil, offset, errors.New("session id is required")
	}
	if validateErr := validateSessionID(sessionID); validateErr != nil {
		return nil, offset, validateErr
	}
	if offset < 0 {
		return nil, offset, errors.New("offset must be >= 0")
	}
	if rootDir == "" {
		var resolveErr error
		rootDir, resolveErr = DefaultDir()
		if resolveErr != nil {
			return nil, offset, resolveErr
		}
	}

	path := filepath.Join(rootDir, sessionID, eventsLiveFileName)
	f, err := os.Open(path) //nolint:gosec // controlled path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}
		return nil, offset, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	stat, err := f.Stat()
	if err != nil {
		return nil, offset, err
	}
	if offset > stat.Size() {
		offset = stat.Size()
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}

	br := bufio.NewReaderSize(f, 64*1024)
	nextOffset = offset
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			// An unterminated trailing line can appear if we race with a writer;
			// keep offset at the prior safe position and retry on the next poll.
			if line[len(line)-1] != '\n' {
				break
			}

			nextOffset += int64(len(line))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				var ev Event
				if err := json.Unmarshal(trimmed, &ev); err == nil {
					events = append(events, ev)
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return events, nextOffset, readErr
		}
	}

	return events, nextOffset, nil
}

// PruneOlderThan removes session directories older than the cutoff.
func PruneOlderThan(rootDir string, cutoff time.Time) (int, error) {
	sessions, err := ListSessions(rootDir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, s := range sessions {
		ref := s.StartedAt
		if s.ClosedAt != nil {
			ref = *s.ClosedAt
		}
		if ref.Before(cutoff) {
			if err := os.RemoveAll(s.Path); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

// DefaultRetention returns the default prune window.
func DefaultRetention() time.Duration {
	return defaultRetentionHours * time.Hour
}

// EventsFileName returns the relative filename for transcript events.
func EventsFileName() string {
	return eventsFileName
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
