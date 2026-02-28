package transcript

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

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
			return nil, fmt.Errorf("resolve transcript root directory: %w", err)
		}
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("list transcript sessions: %w", err)
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
			return nil, fmt.Errorf("resolve transcript root directory: %w", resolveErr)
		}
	}

	gzPath := filepath.Join(rootDir, sessionID, eventsFileName)

	file, err := os.Open(gzPath) //nolint:gosec // controlled path
	if err != nil {
		if os.IsNotExist(err) {
			// Compressed file missing — fall back to live file (crashed session).
			return readEventsFromLiveFile(rootDir, sessionID)
		}

		return nil, fmt.Errorf("open transcript events: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}

	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	scanner := bufio.NewScanner(gzipReader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("scan transcript events: %w", err)
	}

	return events, nil
}

// readEventsFromLiveFile reads events from the plain JSONL live file.
// This is used as a fallback when the compressed file doesn't exist
// (e.g., a crashed session where Close() never ran).
func readEventsFromLiveFile(rootDir, sessionID string) (events []Event, err error) {
	livePath := filepath.Join(rootDir, sessionID, eventsLiveFileName)

	file, err := os.Open(livePath) //nolint:gosec // controlled path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("open live transcript events for recovery: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		trimmed := bytes.TrimSpace(scanner.Bytes())
		if len(trimmed) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(trimmed, &event); err != nil {
			continue
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("scan live transcript events: %w", err)
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
			return nil, offset, fmt.Errorf("resolve transcript root directory: %w", resolveErr)
		}
	}

	path := filepath.Join(rootDir, sessionID, eventsLiveFileName)

	file, err := os.Open(path) //nolint:gosec // controlled path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}

		return nil, offset, fmt.Errorf("open live transcript events: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	stat, err := file.Stat()
	if err != nil {
		return nil, offset, fmt.Errorf("read live transcript file info: %w", err)
	}

	if offset > stat.Size() {
		offset = stat.Size()
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek live transcript file: %w", err)
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	nextOffset = offset

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			// An unterminated trailing line can appear if we race with a writer;
			// keep offset at the prior safe position and retry on the next poll.
			if line[len(line)-1] != '\n' {
				break
			}

			nextOffset += int64(len(line))

			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				var event Event
				if err := json.Unmarshal(trimmed, &event); err == nil {
					events = append(events, event)
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}

			return events, nextOffset, fmt.Errorf("read live transcript line: %w", readErr)
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

	for _, session := range sessions {
		referenceTime := session.StartedAt
		if session.ClosedAt != nil {
			referenceTime = *session.ClosedAt
		}

		if referenceTime.Before(cutoff) {
			if err := os.RemoveAll(session.Path); err != nil {
				return removed, fmt.Errorf("prune transcript session %q: %w", session.SessionID, err)
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
