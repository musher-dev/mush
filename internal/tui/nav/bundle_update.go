package nav

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/bundle"
)

// handleBundleListLoaded processes the async-loaded recent/installed bundle lists.
func (m *model) handleBundleListLoaded(msg bundleListLoadedMsg) (tea.Model, tea.Cmd) {
	m.bundleInput.recentBundles = msg.recent
	m.bundleInput.installedBundles = msg.installed
	m.bundleInput.listLoaded = true

	return m, nil
}

// handleBundleInputKey processes key events on the bundle input screen.
func (m *model) handleBundleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Hotkeys only work when text input is NOT focused.
	if m.bundleInput.focusArea != bundleFocusInput && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'f':
			return m.activateBundleHubLink()
		case 'n':
			return m.activateBareRun()
		}
	}

	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Tab):
		// Cycle: input → list → harness → input.
		switch m.bundleInput.focusArea {
		case bundleFocusInput:
			m.bundleInput.focusArea = bundleFocusList
			m.bundleInput.textInput.Blur()
		case bundleFocusList:
			m.bundleInput.focusArea = bundleFocusHarness
		default:
			m.bundleInput.focusArea = bundleFocusInput
			m.bundleInput.textInput.Focus()
		}

		return m, nil

	case key.Matches(msg, m.keys.Select):
		return m.submitBundleSelection()

	case key.Matches(msg, m.keys.Down):
		switch m.bundleInput.focusArea {
		case bundleFocusList:
			maxIdx := bundleListLen(m) - 1
			if maxIdx < 0 {
				maxIdx = 0
			}

			if m.bundleInput.listCursor < maxIdx {
				m.bundleInput.listCursor++
			}

			return m, nil
		case bundleFocusHarness:
			if m.bundleInput.harnessCur < len(m.harnesses)-1 {
				m.bundleInput.harnessCur++
			}

			return m, nil
		}

	case key.Matches(msg, m.keys.Up):
		switch m.bundleInput.focusArea {
		case bundleFocusList:
			if m.bundleInput.listCursor > 0 {
				m.bundleInput.listCursor--
			}

			return m, nil
		case bundleFocusHarness:
			if m.bundleInput.harnessCur > 0 {
				m.bundleInput.harnessCur--
			}

			return m, nil
		}
	}

	// Forward to text input if focused.
	if m.bundleInput.focusArea == bundleFocusInput {
		var cmd tea.Cmd

		m.bundleInput.textInput, cmd = m.bundleInput.textInput.Update(msg)

		return m, cmd
	}

	return m, nil
}

// submitBundleSelection handles enter on the bundle input screen.
// Delegates to the appropriate handler depending on what's selected.
func (m *model) submitBundleSelection() (tea.Model, tea.Cmd) {
	// If text input is focused, submit the typed slug.
	if m.bundleInput.focusArea == bundleFocusInput {
		return m.submitBundleInput()
	}

	// Otherwise, check what list item is selected.
	cursor := m.bundleInput.listCursor
	recentLen := len(m.bundleInput.recentBundles)
	installedLen := len(m.bundleInput.installedBundles)

	// Recent bundle selected.
	if cursor < recentLen {
		r := m.bundleInput.recentBundles[cursor]
		m.bundleInput.textInput.SetValue(r.namespace + "/" + r.slug + ":" + r.version)
		m.bundleInput.focusArea = bundleFocusInput
		m.bundleInput.textInput.Focus()

		return m.submitBundleInput()
	}

	cursor -= recentLen

	// Installed bundle selected.
	if cursor < installedLen {
		b := m.bundleInput.installedBundles[cursor]
		m.bundleInput.textInput.SetValue(b.slug)
		m.bundleInput.focusArea = bundleFocusInput
		m.bundleInput.textInput.Focus()

		return m.submitBundleInput()
	}

	cursor -= installedLen

	// Action links.
	if cursor == 0 {
		return m.activateBundleHubLink()
	}

	return m.activateBareRun()
}

// activateBundleHubLink navigates to the hub explore screen from the bundle input.
func (m *model) activateBundleHubLink() (tea.Model, tea.Cmd) {
	searchField := textinput.New()
	searchField.Placeholder = "Search bundles..."
	searchField.CharLimit = 128
	searchField.Width = m.styles.hubWidth - 12 //nolint:mnd // panel padding + border
	searchField.Focus()

	m.hubExplore = hubExploreState{
		searchInput: searchField,
		categoryCur: -1,
		loading:     true,
		spinner:     m.hubExplore.spinner,
		searchID:    m.hubExplore.searchID + 1,
	}

	m.pushScreen(screenHubExplore)

	baseURL := m.apiBaseURL()

	return m, tea.Batch(
		m.hubExplore.spinner.Tick,
		cmdSearchHub(baseURL, "", "", "trending", hubSearchLimit, "", false, m.hubExplore.searchID),
		cmdListHubCategories(baseURL),
	)
}

// activateBareRun exits the TUI with a bare harness run (no bundle).
func (m *model) activateBareRun() (tea.Model, tea.Cmd) {
	harness := ""
	if len(m.harnesses) > 0 {
		harness = m.harnesses[m.bundleInput.harnessCur].name
	}

	m.result = &Result{
		Action:  ActionBareRun,
		Harness: harness,
	}

	return m, tea.Quit
}

// submitBundleInput validates the input and starts the resolve flow.
func (m *model) submitBundleInput() (tea.Model, tea.Cmd) {
	raw := m.bundleInput.textInput.Value()

	ref, err := bundle.ParseRef(raw)
	if err != nil {
		m.bundleError = bundleErrorState{
			message: err.Error(),
			hint:    "Enter a bundle reference like 'namespace/slug' or 'namespace/slug:1.0.0'",
			slug:    raw,
			harness: m.harnesses[m.bundleInput.harnessCur].name,
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	// Check if client is available.
	if m.deps == nil || m.deps.Client == nil {
		m.bundleError = bundleErrorState{
			message:   "Not authenticated",
			hint:      "Run 'mush auth login' first to authenticate",
			namespace: ref.Namespace,
			slug:      ref.Slug,
			version:   ref.Version,
			harness:   m.harnesses[m.bundleInput.harnessCur].name,
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	// Start resolving.
	m.bundleResolve = bundleResolveState{
		spinner:   m.bundleResolve.spinner,
		namespace: ref.Namespace,
		slug:      ref.Slug,
		version:   ref.Version,
	}

	harness := m.harnesses[m.bundleInput.harnessCur].name
	m.pushScreen(screenBundleResolving)

	return m, tea.Batch(
		m.bundleResolve.spinner.Tick,
		cmdResolveBundle(m.deps.Client, ref.Namespace, ref.Slug, ref.Version, harness),
	)
}

// handleBundleResolvingKey processes key events on the resolving screen.
func (m *model) handleBundleResolvingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) {
		if m.bundleResolve.cancel != nil {
			m.bundleResolve.cancel()
		}

		m.popScreen()
	}

	return m, nil
}

// handleBundleConfirmKey processes key events on the confirmation screen.
func (m *model) handleBundleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Tab):
		m.bundleConfirm.buttonIdx = (m.bundleConfirm.buttonIdx + 1) % 2 //nolint:mnd // 2 buttons

	case key.Matches(msg, m.keys.Select):
		if m.bundleConfirm.buttonIdx == 1 {
			// Cancel — go back.
			m.popScreen()

			return m, nil
		}

		// Load — start download.
		return m.startBundleDownload()
	}

	return m, nil
}

// startBundleDownload begins the download/cache process.
func (m *model) startBundleDownload() (tea.Model, tea.Cmd) {
	namespace := m.bundleConfirm.namespace
	slug := m.bundleConfirm.slug
	ver := m.bundleConfirm.version
	harness := m.bundleConfirm.harness

	m.bundleProgress = bundleProgressState{
		progress:  m.bundleProgress.progress,
		namespace: namespace,
		slug:      slug,
		version:   ver,
		label:     "Checking cache...",
	}

	m.pushScreen(screenBundleProgress)

	return m, cmdCheckBundleCache(m.deps, namespace, slug, ver, harness)
}

// handleBundleProgressKey processes key events on the progress screen.
func (m *model) handleBundleProgressKey(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// No key actions during download — just wait for completion.
	return m, nil
}

// handleBundleCompleteKey processes key events on the complete screen.
func (m *model) handleBundleCompleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		// Return to home.
		m.screenStack = nil
		m.activeScreen = screenHome

	case key.Matches(msg, m.keys.Select):
		// Launch session — exit TUI with result.
		m.result = &Result{
			Action:     ActionBundleLoad,
			BundleSlug: m.bundleComplete.slug,
			BundleVer:  m.bundleComplete.version,
			Harness:    m.bundleComplete.harness,
			CachePath:  m.bundleComplete.cachePath,
		}

		return m, tea.Quit
	}

	return m, nil
}

// handleBundleErrorKey processes key events on the error screen.
func (m *model) handleBundleErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleErrorScreenKey(msg, &m.bundleError.buttonIdx, m.retryBundleResolve)
}

// retryBundleResolve retries the resolve from the error screen.
func (m *model) retryBundleResolve() (tea.Model, tea.Cmd) {
	namespace := m.bundleError.namespace
	slug := m.bundleError.slug
	version := m.bundleError.version
	harness := m.bundleError.harness

	if m.deps == nil || m.deps.Client == nil {
		return m, nil
	}

	// Parse the slug in case it was the raw input.
	ref, err := bundle.ParseRef(slug)
	if err != nil {
		// If the slug alone doesn't parse (e.g. raw input without namespace),
		// try constructing a ref from the carried-forward namespace.
		if namespace == "" {
			return m, nil
		}

		ref = bundle.Ref{Namespace: namespace, Slug: slug}
	}

	if version != "" {
		ref.Version = version
	}

	m.bundleResolve = bundleResolveState{
		spinner:   m.bundleResolve.spinner,
		namespace: ref.Namespace,
		slug:      ref.Slug,
		version:   ref.Version,
	}

	// Replace the error screen with resolving.
	m.activeScreen = screenBundleResolving

	return m, tea.Batch(
		m.bundleResolve.spinner.Tick,
		cmdResolveBundle(m.deps.Client, ref.Namespace, ref.Slug, ref.Version, harness),
	)
}

// handleBundleResolved processes a successful resolve.
func (m *model) handleBundleResolved(msg *bundleResolvedMsg) (tea.Model, tea.Cmd) {
	m.bundleConfirm = bundleConfirmState{
		namespace:  msg.namespace,
		slug:       msg.slug,
		version:    msg.version,
		assetCount: msg.assetCount,
		harness:    msg.harness,
		buttonIdx:  0,
	}

	// Replace resolving screen with confirm.
	m.activeScreen = screenBundleConfirm

	return m, nil
}

// handleBundleResolveError processes a resolve error.
func (m *model) handleBundleResolveError(msg bundleResolveErrorMsg) (tea.Model, tea.Cmd) {
	m.bundleError = bundleErrorState{
		message:   msg.err.Error(),
		hint:      "Check the bundle reference and try again",
		namespace: m.bundleResolve.namespace,
		slug:      msg.slug,
		version:   msg.version,
		harness:   msg.harness,
	}

	// Replace resolving screen with error.
	m.activeScreen = screenBundleError

	return m, nil
}

// handleBundleCacheHit processes a cache hit.
func (m *model) handleBundleCacheHit(msg bundleCacheHitMsg) (tea.Model, tea.Cmd) {
	m.bundleComplete = bundleCompleteState{
		namespace: m.bundleProgress.namespace,
		slug:      m.bundleProgress.slug,
		version:   m.bundleProgress.version,
		harness:   msg.harness,
		cachePath: msg.cachePath,
	}

	// Replace progress screen with complete.
	m.activeScreen = screenBundleComplete

	return m, nil
}

// handleBundleDownloadProgress processes a download progress update.
func (m *model) handleBundleDownloadProgress(msg bundleDownloadProgressMsg) (tea.Model, tea.Cmd) {
	m.bundleProgress.current = msg.current
	m.bundleProgress.total = msg.total
	m.bundleProgress.label = msg.label

	return m, nil
}

// handleBundleDownloadComplete processes download completion.
func (m *model) handleBundleDownloadComplete(msg bundleDownloadCompleteMsg) (tea.Model, tea.Cmd) {
	m.bundleComplete = bundleCompleteState{
		namespace: m.bundleProgress.namespace,
		slug:      m.bundleProgress.slug,
		version:   m.bundleProgress.version,
		harness:   msg.harness,
		cachePath: msg.cachePath,
	}

	// Replace progress screen with complete.
	m.activeScreen = screenBundleComplete

	return m, nil
}

// handleBundleDownloadError processes a download error.
func (m *model) handleBundleDownloadError(msg bundleDownloadErrorMsg) (tea.Model, tea.Cmd) {
	m.bundleError = bundleErrorState{
		message:   msg.err.Error(),
		hint:      "Check your connection and try again",
		namespace: m.bundleProgress.namespace,
		slug:      m.bundleProgress.slug,
		version:   m.bundleProgress.version,
		harness:   msg.harness,
	}

	// Replace progress screen with error.
	m.activeScreen = screenBundleError

	return m, nil
}
