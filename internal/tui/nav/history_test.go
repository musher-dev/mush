package nav

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/transcript"
)

func TestHistoryHotkeyActivatesScreen(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if mdl.activeScreen != screenHistory {
		t.Errorf("activeScreen = %d, want screenHistory", mdl.activeScreen)
	}

	if !mdl.history.loading {
		t.Error("history.loading should be true after hotkey activation")
	}

	if mdl.cursor != 5 {
		t.Errorf("cursor = %d, want 5 (View history)", mdl.cursor)
	}
}

func TestHistorySessionsLoadedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = true

	now := time.Now()

	msg := historySessionsLoadedMsg{
		sessions: []transcript.Session{
			{SessionID: "abc12345", StartedAt: now},
			{SessionID: "def67890", StartedAt: now.Add(-time.Hour)},
		},
	}

	mdl = updateModel(mdl, msg)

	if mdl.history.loading {
		t.Error("history.loading should be false after completion")
	}

	if len(mdl.history.sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(mdl.history.sessions))
	}
}

func TestHistorySessionsLoadedError(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = true

	msg := historySessionsLoadedMsg{
		err: fmt.Errorf("disk error"),
	}

	mdl = updateModel(mdl, msg)

	if mdl.history.loading {
		t.Error("history.loading should be false after error")
	}

	if mdl.history.errorMsg != "disk error" {
		t.Errorf("errorMsg = %q, want 'disk error'", mdl.history.errorMsg)
	}
}

func TestHistoryStaleSessionsMsgIgnored(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHome // Not on history screen.

	msg := historySessionsLoadedMsg{
		sessions: []transcript.Session{
			{SessionID: "abc12345", StartedAt: time.Now()},
		},
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.history.sessions) != 0 {
		t.Errorf("sessions should be empty when on different screen, got %d", len(mdl.history.sessions))
	}
}

func TestHistoryEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenHistory)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome", mdl.activeScreen)
	}
}

func TestHistoryCursorDown(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = []transcript.Session{
		{SessionID: "aaa"},
		{SessionID: "bbb"},
		{SessionID: "ccc"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.history.cursor != 1 {
		t.Errorf("cursor = %d, want 1 after down", mdl.history.cursor)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.history.cursor != 2 {
		t.Errorf("cursor = %d, want 2 after second down", mdl.history.cursor)
	}

	// Clamp at end.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.history.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", mdl.history.cursor)
	}
}

func TestHistoryCursorUp(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = []transcript.Session{
		{SessionID: "aaa"},
		{SessionID: "bbb"},
	}
	mdl.history.cursor = 1

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.history.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after up", mdl.history.cursor)
	}

	// Clamp at start.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.history.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", mdl.history.cursor)
	}
}

func TestHistoryEnterViewsDetail(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = []transcript.Session{
		{SessionID: "abc12345-full-id", StartedAt: time.Now()},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHistoryDetail {
		t.Errorf("activeScreen = %d, want screenHistoryDetail", mdl.activeScreen)
	}

	if mdl.historyDetail.session.SessionID != "abc12345-full-id" {
		t.Errorf("session.SessionID = %q, want abc12345-full-id", mdl.historyDetail.session.SessionID)
	}

	if !mdl.historyDetail.loading {
		t.Error("historyDetail.loading should be true after entering detail")
	}
}

func TestHistoryEnterEmptyListNoop(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = nil

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHistory {
		t.Errorf("activeScreen = %d, want screenHistory (no-op on empty list)", mdl.activeScreen)
	}
}

func TestHistoryRefreshReloads(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = false
	mdl.history.sessions = []transcript.Session{
		{SessionID: "old"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.history.loading {
		t.Error("history.loading should be true after refresh")
	}

	if mdl.history.sessions != nil {
		t.Error("sessions should be nil during refresh")
	}
}

func TestHistoryRefreshIgnoredDuringLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = true

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !mdl.history.loading {
		t.Error("history.loading should remain true when refresh pressed during loading")
	}
}

func TestHistoryDetailEscGoesBackToList(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenHistory)
	mdl.pushScreen(screenHistoryDetail)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHistory {
		t.Errorf("activeScreen = %d, want screenHistory after esc from detail", mdl.activeScreen)
	}
}

func TestHistoryEventsLoadedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistoryDetail
	mdl.historyDetail.loading = true

	msg := historyEventsLoadedMsg{
		sessionID: "abc12345",
		events: []transcript.Event{
			{SessionID: "abc12345", Text: "hello world"},
		},
		lines: []string{"hello world"},
	}

	mdl = updateModel(mdl, msg)

	if mdl.historyDetail.loading {
		t.Error("historyDetail.loading should be false after events loaded")
	}

	if len(mdl.historyDetail.lines) != 1 {
		t.Errorf("lines = %d, want 1", len(mdl.historyDetail.lines))
	}

	if len(mdl.historyDetail.events) != 1 {
		t.Errorf("events = %d, want 1", len(mdl.historyDetail.events))
	}
}

func TestHistoryDetailScrollDown(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistoryDetail

	// Create enough lines to scroll.
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}

	mdl.historyDetail.lines = lines

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.historyDetail.scrollOffset != 1 {
		t.Errorf("scrollOffset = %d, want 1 after down", mdl.historyDetail.scrollOffset)
	}
}

func TestHistoryDetailScrollUpClamp(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistoryDetail
	mdl.historyDetail.lines = []string{"line 1"}
	mdl.historyDetail.scrollOffset = 0

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.historyDetail.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 (clamped at 0)", mdl.historyDetail.scrollOffset)
	}
}

func TestHistoryListViewLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = true

	view := mdl.View()

	if !strings.Contains(view, "Loading sessions") {
		t.Error("view should contain 'Loading sessions' during loading")
	}

	if !strings.Contains(view, "Sessions") {
		t.Error("view should contain 'Sessions' panel title")
	}
}

func TestHistoryListViewWithSessions(t *testing.T) {
	t.Parallel()

	now := time.Now()
	closed := now.Add(2 * time.Minute) //nolint:mnd // 2 min duration for test

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = []transcript.Session{
		{SessionID: "abc12345-long-id", StartedAt: now, ClosedAt: &closed},
		{SessionID: "def67890-open-id", StartedAt: now},
	}

	view := mdl.View()

	if !strings.Contains(view, "Sessions") {
		t.Error("view should contain 'Sessions' panel title")
	}

	if !strings.Contains(view, "abc12345") {
		t.Error("view should contain truncated session ID")
	}

	if !strings.Contains(view, "2m") {
		t.Error("view should contain duration for closed session")
	}

	if !strings.Contains(view, "running") {
		t.Error("view should contain 'running' for open session")
	}

	if !strings.Contains(view, "2 sessions") {
		t.Error("view should contain session count summary")
	}

	if !strings.Contains(view, "1 open") {
		t.Error("view should contain open count in summary")
	}
}

func TestHistoryListViewEmpty(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.loading = false

	view := mdl.View()

	if !strings.Contains(view, "No transcript sessions found") {
		t.Error("view should contain empty state message")
	}
}

func TestHistoryDetailViewLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHistoryDetail
	mdl.historyDetail.loading = true

	view := mdl.View()

	if !strings.Contains(view, "Loading events") {
		t.Error("view should contain 'Loading events' during loading")
	}
}

func TestHistoryDetailViewEmpty(t *testing.T) {
	t.Parallel()

	now := time.Now()
	closed := now.Add(30 * time.Second) //nolint:mnd // 30s duration for test

	mdl := testModel()
	mdl.activeScreen = screenHistoryDetail
	mdl.historyDetail.loading = false
	mdl.historyDetail.session = transcript.Session{
		SessionID: "abc12345-full-id",
		StartedAt: now,
		ClosedAt:  &closed,
	}

	view := mdl.View()

	if !strings.Contains(view, "No events recorded") {
		t.Error("view should contain 'No events recorded' for empty session")
	}

	if !strings.Contains(view, "abc12345") {
		t.Error("view should contain truncated session ID in header")
	}

	if !strings.Contains(view, "30s") {
		t.Error("view should contain duration in header")
	}

	if !strings.Contains(view, "0 events") {
		t.Error("view should contain event count in header")
	}
}

func TestHistoryStaleEventsIgnored(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHome // Not on detail screen.
	mdl.historyDetail.loading = true

	msg := historyEventsLoadedMsg{
		sessionID: "abc12345",
		lines:     []string{"should be ignored"},
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.historyDetail.lines) != 0 {
		t.Errorf("lines should be empty when on different screen, got %d", len(mdl.historyDetail.lines))
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "<1s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m 30s"},
		{2 * time.Minute, "2m"},
		{5*time.Minute + 15*time.Second, "5m 15s"},
		{time.Hour, "1h"},
		{time.Hour + 30*time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}

	for _, test := range tests {
		got := formatDuration(test.d)
		if got != test.want {
			t.Errorf("formatDuration(%v) = %q, want %q", test.d, got, test.want)
		}
	}
}

func TestHistoryListViewAllClosed(t *testing.T) {
	t.Parallel()

	now := time.Now()
	closed := now.Add(10 * time.Second) //nolint:mnd // 10s for test

	mdl := testModel()
	mdl.activeScreen = screenHistory
	mdl.history.sessions = []transcript.Session{
		{SessionID: "aaa11111", StartedAt: now, ClosedAt: &closed},
	}

	view := mdl.View()

	// When all sessions are closed, summary should not mention "open".
	if strings.Contains(view, "open") {
		t.Error("view should not mention 'open' when all sessions are closed")
	}

	if !strings.Contains(view, "1 sessions") {
		t.Error("view should contain session count")
	}
}
