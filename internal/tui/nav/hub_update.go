package nav

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// handleHubExploreKey processes key events on the hub explore screen.
func (m *model) handleHubExploreKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.hubExplore.focusArea = (m.hubExplore.focusArea + 1) % buttonCount

		if m.hubExplore.focusArea == 0 {
			m.hubExplore.searchInput.Focus()
		} else {
			m.hubExplore.searchInput.Blur()
		}

		return m, nil
	}

	switch m.hubExplore.focusArea {
	case 0:
		return m.handleHubSearchKey(msg)
	case 1:
		return m.handleHubListKey(msg)
	}

	return m, nil
}

// handleHubSearchKey handles key events when search input is focused.
func (m *model) handleHubSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Select) {
		query := m.hubExplore.searchInput.Value()
		m.hubExplore.query = query
		m.hubExplore.loading = true
		m.hubExplore.results = nil
		m.hubExplore.resultCur = 0
		m.hubExplore.nextCursor = ""
		m.hubExplore.searchID++

		baseURL := m.apiBaseURL()

		return m, tea.Batch(
			m.hubExplore.spinner.Tick,
			cmdSearchHub(m.ctx, baseURL, query, "", "trending", hubSearchLimit, "", false, m.hubExplore.searchID),
		)
	}

	// Forward to text input.
	var cmd tea.Cmd

	m.hubExplore.searchInput, cmd = m.hubExplore.searchInput.Update(msg)

	// Schedule debounce if text changed.
	newVal := m.hubExplore.searchInput.Value()
	if newVal != m.hubExplore.pendingQuery {
		m.hubExplore.pendingQuery = newVal
		m.hubExplore.debounceID++

		debounceCmd := cmdHubDebounceTick(m.hubExplore.debounceID, newVal)

		return m, tea.Batch(cmd, debounceCmd)
	}

	return m, cmd
}

// handleHubListKey handles key events when the results list is focused.
func (m *model) handleHubListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.hubExplore.resultCur < len(m.hubExplore.results)-1 {
			m.hubExplore.resultCur++
		}

	case key.Matches(msg, m.keys.Up):
		if m.hubExplore.resultCur > 0 {
			m.hubExplore.resultCur--
		}

	case key.Matches(msg, m.keys.Select):
		return m.hubViewDetail()

	default:
		switch {
		case key.Matches(msg, m.keys.Retry):
			return m.hubRunSelected()
		case key.Matches(msg, m.keys.Search):
			m.hubExplore.focusArea = 0
			m.hubExplore.searchInput.Focus()

			return m, nil
		case key.Matches(msg, m.keys.LoadMore):
			if m.hubExplore.hasMore && !m.hubExplore.loading {
				return m.hubLoadMore()
			}
		}
	}

	return m, nil
}

// handleHubDetailKey processes key events on the hub detail screen.
func (m *model) handleHubDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Select):
		return m.hubRunFromDetail()

	case key.Matches(msg, m.keys.Down):
		// Clamp to avoid scrolling past content.
		maxScroll := m.hubDetailContentLineCount()
		if m.hubDetail.scrollOffset < maxScroll {
			m.hubDetail.scrollOffset++
		}

	case key.Matches(msg, m.keys.Up):
		if m.hubDetail.scrollOffset > 0 {
			m.hubDetail.scrollOffset--
		}

	default:
		if key.Matches(msg, m.keys.Retry) {
			return m.hubRunFromDetail()
		}
	}

	return m, nil
}

// --- Hub message handlers ---

func (m *model) handleHubSearchResult(msg hubSearchResultMsg) (tea.Model, tea.Cmd) {
	// Discard out-of-order results from a stale search.
	if msg.searchID != m.hubExplore.searchID {
		return m, nil
	}

	m.hubExplore.loading = false
	m.hubExplore.errorMsg = ""

	if msg.appendMore {
		m.hubExplore.results = append(m.hubExplore.results, msg.results...)
	} else {
		m.hubExplore.results = msg.results
		m.hubExplore.resultCur = 0
	}

	m.hubExplore.nextCursor = msg.nextCursor
	m.hubExplore.hasMore = msg.hasMore
	m.hubExplore.query = msg.query

	return m, nil
}

func (m *model) handleHubSearchError(msg hubSearchErrorMsg) (tea.Model, tea.Cmd) {
	// Discard out-of-order errors from a stale search.
	if msg.searchID != m.hubExplore.searchID {
		return m, nil
	}

	m.hubExplore.loading = false
	m.hubExplore.errorMsg = msg.err.Error()

	return m, nil
}

func (m *model) handleHubDetailLoaded(msg hubDetailLoadedMsg) (tea.Model, tea.Cmd) {
	m.hubDetail.loading = false
	m.hubDetail.detail = msg.detail
	m.hubDetail.errorMsg = ""

	return m, nil
}

func (m *model) handleHubDetailError(msg hubDetailErrorMsg) (tea.Model, tea.Cmd) {
	m.hubDetail.loading = false
	m.hubDetail.errorMsg = msg.err.Error()

	return m, nil
}

func (m *model) handleHubDebounceTick(msg hubDebounceTickMsg) (tea.Model, tea.Cmd) {
	// Ignore stale ticks.
	if msg.id != m.hubExplore.debounceID {
		return m, nil
	}

	// Ignore if query hasn't changed.
	if msg.query == m.hubExplore.query {
		return m, nil
	}

	m.hubExplore.query = msg.query
	m.hubExplore.loading = true
	m.hubExplore.results = nil
	m.hubExplore.resultCur = 0
	m.hubExplore.nextCursor = ""
	m.hubExplore.searchID++

	baseURL := m.apiBaseURL()

	return m, tea.Batch(
		m.hubExplore.spinner.Tick,
		cmdSearchHub(m.ctx, baseURL, msg.query, "", "trending", hubSearchLimit, "", false, m.hubExplore.searchID),
	)
}

// --- Hub helpers ---

// hubDetailContentLineCount returns the approximate number of lines in the detail content.
// Used to clamp scrollOffset.
func (m *model) hubDetailContentLineCount() int {
	if m.hubDetail.detail == nil {
		return 0
	}

	// Count: name, ref+badge, blank, up to 5 meta rows, blank, desc, blank, button = ~12 lines.
	// This is an approximation — we count the same fields renderHubDetailContent renders.
	count := 3 // name + ref + blank

	metaFields := []string{
		m.hubDetail.detail.LatestVersion,
		m.hubDetail.detail.License,
	}

	for _, field := range metaFields {
		if field != "" {
			count++
		}
	}

	if m.hubDetail.detail.StarsCount > 0 {
		count++
	}

	if m.hubDetail.detail.DownloadsTotal > 0 {
		count++
	}

	desc := m.hubDetail.detail.Description
	if desc == "" {
		desc = m.hubDetail.detail.Summary
	}

	if desc != "" {
		count += 2 // blank + desc
	}

	count += 2 // blank + button

	return count
}

// apiBaseURL returns the configured API base URL, with a fallback default.
func (m *model) apiBaseURL() string {
	if m.deps != nil && m.deps.Config != nil {
		return m.deps.Config.APIURL()
	}

	return "https://api.musher.dev"
}

// hubViewDetail navigates to the detail screen for the selected bundle.
func (m *model) hubViewDetail() (tea.Model, tea.Cmd) {
	if len(m.hubExplore.results) == 0 {
		return m, nil
	}

	selected := m.hubExplore.results[m.hubExplore.resultCur]

	m.hubDetail = hubDetailState{
		loading:   true,
		spinner:   m.hubDetail.spinner,
		publisher: selected.Publisher.Handle,
		slug:      selected.Slug,
	}

	m.pushScreen(screenHubDetail)

	baseURL := m.apiBaseURL()

	return m, tea.Batch(
		m.hubDetail.spinner.Tick,
		cmdGetHubDetail(m.ctx, baseURL, selected.Publisher.Handle, selected.Slug),
	)
}

// hubRunSelected starts the load flow for the selected bundle in the list.
func (m *model) hubRunSelected() (tea.Model, tea.Cmd) {
	if len(m.hubExplore.results) == 0 {
		return m, nil
	}

	selected := m.hubExplore.results[m.hubExplore.resultCur]

	if selected.LatestVersion == "" {
		m.bundleError = bundleErrorState{
			message:   "No published versions",
			hint:      "This bundle has no published versions yet — check back later",
			namespace: selected.Publisher.Handle,
			slug:      selected.Slug,
			buttonIdx: 1, // Default to Back — retry would hit the same 404.
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	return m.hubRun(selected.Publisher.Handle, selected.Slug, selected.LatestVersion)
}

// hubRunFromDetail starts the load flow from the detail screen.
func (m *model) hubRunFromDetail() (tea.Model, tea.Cmd) {
	if m.hubDetail.detail == nil {
		return m, nil
	}

	detail := m.hubDetail.detail

	if detail.LatestVersion == "" {
		m.bundleError = bundleErrorState{
			message:   "No published versions",
			hint:      "This bundle has no published versions yet — check back later",
			namespace: detail.Publisher.Handle,
			slug:      detail.Slug,
			buttonIdx: 1, // Default to Back — retry would hit the same 404.
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	return m.hubRun(detail.Publisher.Handle, detail.Slug, detail.LatestVersion)
}

// hubRun bridges hub selection to the existing bundle resolve flow.
func (m *model) hubRun(namespace, slug, version string) (tea.Model, tea.Cmd) {
	if m.deps == nil || m.deps.Client == nil {
		// Should not happen — anonymous client is created at startup.
		m.bundleError = bundleErrorState{
			message:   "Unable to connect",
			hint:      "Client not available — try restarting mush",
			namespace: namespace,
			slug:      slug,
			version:   version,
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	m.bundleResolve = bundleResolveState{
		spinner:   m.bundleResolve.spinner,
		namespace: namespace,
		slug:      slug,
		version:   version,
	}

	m.pushScreen(screenBundleResolving)

	return m, tea.Batch(
		m.bundleResolve.spinner.Tick,
		cmdResolveBundle(m.ctx, m.deps.Client, namespace, slug, version),
	)
}

// hubLoadMore loads the next page of search results.
func (m *model) hubLoadMore() (tea.Model, tea.Cmd) {
	m.hubExplore.loading = true
	m.hubExplore.searchID++

	baseURL := m.apiBaseURL()

	return m, tea.Batch(
		m.hubExplore.spinner.Tick,
		cmdSearchHub(m.ctx, baseURL, m.hubExplore.query, "", "trending", hubSearchLimit, m.hubExplore.nextCursor, true, m.hubExplore.searchID),
	)
}
