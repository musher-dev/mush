package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleHistoryListKey processes key events on the history list screen.
func (m *model) handleHistoryListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Down):
		if !m.history.loading && m.history.cursor < len(m.history.sessions)-1 {
			m.history.cursor++
		}

		return m, nil

	case key.Matches(msg, m.keys.Up):
		if !m.history.loading && m.history.cursor > 0 {
			m.history.cursor--
		}

		return m, nil

	case key.Matches(msg, m.keys.Select):
		if m.history.loading || len(m.history.sessions) == 0 {
			return m, nil
		}

		return m.historyViewDetail()

	case key.Matches(msg, m.keys.Retry):
		if m.history.loading {
			return m, nil
		}

		m.history = historyListState{
			spinner: m.history.spinner,
			loading: true,
		}

		return m, tea.Batch(m.history.spinner.Tick, cmdLoadHistorySessions())
	}

	return m, nil
}

// handleHistoryDetailKey processes key events on the history detail screen.
func (m *model) handleHistoryDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Down):
		if !m.historyDetail.loading {
			maxScroll := m.historyDetailMaxScroll()
			if m.historyDetail.scrollOffset < maxScroll {
				m.historyDetail.scrollOffset++
			}
		}

		return m, nil

	case key.Matches(msg, m.keys.Up):
		if !m.historyDetail.loading && m.historyDetail.scrollOffset > 0 {
			m.historyDetail.scrollOffset--
		}

		return m, nil
	}

	return m, nil
}

// handleHistorySessionsLoaded processes the result of listing transcript sessions.
func (m *model) handleHistorySessionsLoaded(msg historySessionsLoadedMsg) (tea.Model, tea.Cmd) {
	if m.activeScreen != screenHistory {
		return m, nil
	}

	m.history.loading = false

	if msg.err != nil {
		m.history.errorMsg = msg.err.Error()

		return m, nil
	}

	m.history.sessions = msg.sessions

	return m, nil
}

// handleHistoryEventsLoaded processes the result of reading transcript events.
func (m *model) handleHistoryEventsLoaded(msg *historyEventsLoadedMsg) (tea.Model, tea.Cmd) {
	if m.activeScreen != screenHistoryDetail {
		return m, nil
	}

	m.historyDetail.loading = false

	if msg.err != nil {
		m.historyDetail.errorMsg = msg.err.Error()

		return m, nil
	}

	m.historyDetail.events = msg.events
	m.historyDetail.lines = msg.lines

	return m, nil
}

// historyViewDetail transitions from the history list to the detail view.
func (m *model) historyViewDetail() (tea.Model, tea.Cmd) {
	session := m.history.sessions[m.history.cursor]

	m.historyDetail = historyDetailState{
		spinner: m.historyDetail.spinner,
		loading: true,
		session: session,
	}

	m.pushScreen(screenHistoryDetail)

	return m, tea.Batch(m.historyDetail.spinner.Tick, cmdLoadHistoryEvents(session.SessionID))
}

// historyDetailMaxScroll computes the maximum scroll offset for the detail view.
func (m *model) historyDetailMaxScroll() int {
	// Reserve lines for chrome: breadcrumb(1) + gap(1) + panel border/padding(5) + header(2) + gap(1) + footer(2).
	chrome := historyChromeLines
	visible := m.height - chrome

	if visible < 1 {
		visible = 1
	}

	maxScroll := len(m.historyDetail.lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}

	return maxScroll
}
