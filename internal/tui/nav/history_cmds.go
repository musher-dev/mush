package nav

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/transcript"
)

// historySessionsLoadedMsg carries the result of listing transcript sessions.
type historySessionsLoadedMsg struct {
	sessions []transcript.Session
	err      error
}

// historyEventsLoadedMsg carries the result of reading transcript events.
type historyEventsLoadedMsg struct {
	sessionID string
	events    []transcript.Event
	lines     []string // ANSI-stripped display lines
	err       error
}

// cmdLoadHistorySessions lists all transcript sessions asynchronously.
func cmdLoadHistorySessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := transcript.ListSessions("")

		return historySessionsLoadedMsg{
			sessions: sessions,
			err:      err,
		}
	}
}

// cmdLoadHistoryEvents reads events for a session and pre-processes them into display lines.
func cmdLoadHistoryEvents(sessionID string) tea.Cmd {
	return func() tea.Msg {
		events, err := transcript.ReadEvents("", sessionID)
		if err != nil {
			return historyEventsLoadedMsg{
				sessionID: sessionID,
				err:       err,
			}
		}

		var lines []string

		for _, ev := range events {
			text := ev.Text
			if text == "" {
				continue
			}

			stripped := ansi.Strip(text)
			lines = append(lines, strings.Split(stripped, "\n")...)
		}

		return historyEventsLoadedMsg{
			sessionID: sessionID,
			events:    events,
			lines:     lines,
		}
	}
}
