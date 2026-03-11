package nav

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
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
		if msg.Runes[0] == 'f' {
			return m.activateBundleHubLink()
		}
	}

	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

		return m, nil

	case key.Matches(msg, m.keys.Tab):
		// Cycle: input → list → myBundles (if two-panel + has content) → input.
		hasMyBundles := m.styles.layout == layoutTwoPanel && len(m.myBundles.bundles) > 0

		switch m.bundleInput.focusArea {
		case bundleFocusInput:
			m.bundleInput.focusArea = bundleFocusList
			m.bundleInput.textInput.Blur()
		case bundleFocusList:
			if hasMyBundles {
				m.bundleInput.focusArea = bundleFocusMyBundles
			} else {
				m.bundleInput.focusArea = bundleFocusInput
				m.bundleInput.textInput.Focus()
			}
		default:
			m.bundleInput.focusArea = bundleFocusInput
			m.bundleInput.textInput.Focus()
		}

		return m, nil

	case key.Matches(msg, m.keys.Select):
		return m.submitBundleSelection()

	case key.Matches(msg, m.keys.Down):
		if m.bundleInput.focusArea == bundleFocusList {
			maxIdx := bundleListLen(m) - 1
			if maxIdx < 0 {
				maxIdx = 0
			}

			if m.bundleInput.listCursor < maxIdx {
				m.bundleInput.listCursor++
			}

			return m, nil
		}

		if m.bundleInput.focusArea == bundleFocusMyBundles && len(m.myBundles.bundles) > 0 {
			if m.myBundles.cursor < len(m.myBundles.bundles)-1 {
				m.myBundles.cursor++
			}

			return m, nil
		}

	case key.Matches(msg, m.keys.Up):
		if m.bundleInput.focusArea == bundleFocusList {
			if m.bundleInput.listCursor > 0 {
				m.bundleInput.listCursor--
			}

			return m, nil
		}

		if m.bundleInput.focusArea == bundleFocusMyBundles {
			if m.myBundles.cursor > 0 {
				m.myBundles.cursor--
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

	// My Bundles panel selected — populate input with the selected bundle and submit.
	if m.bundleInput.focusArea == bundleFocusMyBundles && len(m.myBundles.bundles) > 0 {
		b := m.myBundles.bundles[m.myBundles.cursor]

		value := b.Publisher.Handle + "/" + b.Slug
		if b.LatestVersion != "" {
			value += ":" + b.LatestVersion
		}

		m.bundleInput.textInput.SetValue(value)
		m.bundleInput.focusArea = bundleFocusInput
		m.bundleInput.textInput.Focus()

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

		value := b.ref
		if b.version != "" {
			value += ":" + b.version
		}

		m.bundleInput.textInput.SetValue(value)
		m.bundleInput.focusArea = bundleFocusInput
		m.bundleInput.textInput.Focus()

		return m.submitBundleInput()
	}

	// Action link: "Find a bundle on the Hub".
	return m.activateBundleHubLink()
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
		loading:     true,
		spinner:     m.hubExplore.spinner,
		searchID:    m.hubExplore.searchID + 1,
	}

	m.pushScreen(screenHubExplore)

	baseURL := m.apiBaseURL()

	return m, tea.Batch(
		m.hubExplore.spinner.Tick,
		cmdSearchHub(m.ctx, baseURL, "", "", "trending", hubSearchLimit, "", false, m.hubExplore.searchID),
	)
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
		}

		m.pushScreen(screenBundleError)

		return m, nil
	}

	// Check if client is available (should always be non-nil; anonymous client is created at startup).
	if m.deps == nil || m.deps.Client == nil {
		m.bundleError = bundleErrorState{
			message:   "Unable to connect",
			hint:      "Client not available — try restarting mush",
			namespace: ref.Namespace,
			slug:      ref.Slug,
			version:   ref.Version,
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

	m.pushScreen(screenBundleResolving)

	return m, tea.Batch(
		m.bundleResolve.spinner.Tick,
		cmdResolveBundle(m.ctx, m.deps.Client, ref.Namespace, ref.Slug, ref.Version),
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

// startBundleDownload begins the download/cache process.
func (m *model) startBundleDownload() (tea.Model, tea.Cmd) {
	namespace := m.bundleConfirm.namespace
	slug := m.bundleConfirm.slug
	ver := m.bundleConfirm.version

	m.bundleProgress = bundleProgressState{
		progress:  m.bundleProgress.progress,
		namespace: namespace,
		slug:      slug,
		version:   ver,
		label:     "Checking cache...",
	}

	m.pushScreen(screenBundleProgress)

	return m, cmdCheckBundleCache(m.ctx, m.deps, namespace, slug, ver)
}

// handleBundleProgressKey processes key events on the progress screen.
func (m *model) handleBundleProgressKey(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// No key actions during download — just wait for completion.
	return m, nil
}

// handleBundleActionKey processes key events on the Run/Install action choice screen.
func (m *model) handleBundleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		// Return to home.
		m.screenStack = nil
		m.activeScreen = screenHome

	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Right):
		m.bundleAction.buttonIdx = (m.bundleAction.buttonIdx + 1) % 2 //nolint:mnd // 2 buttons

	case key.Matches(msg, m.keys.Select):
		// Build list of installed harness indices.
		var installed []int

		for i, h := range m.harnesses {
			for _, s := range m.homeHarness.statuses {
				if s.name == h.name && s.installed {
					installed = append(installed, i)

					break
				}
			}
		}

		// Fall back to all harnesses if none detected as installed.
		if len(installed) == 0 {
			for i := range m.harnesses {
				installed = append(installed, i)
			}
		}

		// Push harness selection screen.
		m.bundleHarness = bundleHarnessState{
			namespace:  m.bundleAction.namespace,
			slug:       m.bundleAction.slug,
			version:    m.bundleAction.version,
			cachePath:  m.bundleAction.cachePath,
			installed:  installed,
			forInstall: m.bundleAction.buttonIdx == 1,
		}

		m.pushScreen(screenBundleHarness)
	}

	return m, nil
}

// handleBundleHarnessKey processes key events on the harness selection screen.
func (m *model) handleBundleHarnessKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	installedCount := len(m.bundleHarness.installed)

	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Down):
		if m.bundleHarness.cursor < installedCount-1 {
			m.bundleHarness.cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.bundleHarness.cursor > 0 {
			m.bundleHarness.cursor--
		}

	case key.Matches(msg, m.keys.Select):
		harness := ""

		if installedCount > 0 && m.bundleHarness.cursor < installedCount {
			idx := m.bundleHarness.installed[m.bundleHarness.cursor]
			harness = m.harnesses[idx].name
		}

		if !m.bundleHarness.forInstall {
			// Run path — exit TUI with ActionBundleLoad.
			m.result = &Result{
				Action:          ActionBundleLoad,
				BundleNamespace: m.bundleHarness.namespace,
				BundleSlug:      m.bundleHarness.slug,
				BundleVer:       m.bundleHarness.version,
				Harness:         harness,
				CachePath:       m.bundleHarness.cachePath,
			}

			return m, tea.Quit
		}

		// Install path — push install confirmation screen.
		workDir := ""
		if m.deps != nil {
			workDir = m.deps.WorkDir
		}

		m.bundleInstallConfirm = bundleInstallConfirmState{
			namespace: m.bundleHarness.namespace,
			slug:      m.bundleHarness.slug,
			version:   m.bundleHarness.version,
			cachePath: m.bundleHarness.cachePath,
			harness:   harness,
			targetDir: workDir,
		}

		m.pushScreen(screenBundleInstallConfirm)

		return m, cmdCheckInstallConflicts(m.bundleHarness.cachePath, harness, workDir)
	}

	return m, nil
}

// handleBundleInstallConfirmKey processes key events on the install confirm screen.
func (m *model) handleBundleInstallConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Tab):
		if m.bundleInstallConfirm.hasConflicts {
			// Cycle: force toggle → Install → Cancel → force toggle.
			maxIdx := 2 //nolint:mnd // 3 focusable areas
			m.bundleInstallConfirm.buttonIdx = (m.bundleInstallConfirm.buttonIdx + 1) % (maxIdx + 1)
		} else {
			// Cycle: Install → Cancel.
			m.bundleInstallConfirm.buttonIdx = (m.bundleInstallConfirm.buttonIdx + 1) % 2 //nolint:mnd // 2 buttons
		}

	case msg.Type == tea.KeySpace:
		// Toggle force if on toggle (buttonIdx == 0 when hasConflicts).
		if m.bundleInstallConfirm.hasConflicts && m.bundleInstallConfirm.buttonIdx == 0 {
			m.bundleInstallConfirm.force = !m.bundleInstallConfirm.force
		}

	case key.Matches(msg, m.keys.Select):
		installBtnIdx := 0

		if m.bundleInstallConfirm.hasConflicts {
			// Toggle force on enter when on toggle.
			if m.bundleInstallConfirm.buttonIdx == 0 {
				m.bundleInstallConfirm.force = !m.bundleInstallConfirm.force

				return m, nil
			}

			installBtnIdx = 1
		}

		if m.bundleInstallConfirm.buttonIdx == installBtnIdx {
			// Install — exit TUI with ActionBundleInstall.
			m.result = &Result{
				Action:          ActionBundleInstall,
				BundleNamespace: m.bundleInstallConfirm.namespace,
				BundleSlug:      m.bundleInstallConfirm.slug,
				BundleVer:       m.bundleInstallConfirm.version,
				Harness:         m.bundleInstallConfirm.harness,
				CachePath:       m.bundleInstallConfirm.cachePath,
				Force:           m.bundleInstallConfirm.force,
			}

			return m, tea.Quit
		}

		// Cancel — pop back.
		m.popScreen()
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

	if m.deps == nil || m.deps.Client == nil {
		// Should not happen — anonymous client is created at startup.
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
		cmdResolveBundle(m.ctx, m.deps.Client, ref.Namespace, ref.Slug, ref.Version),
	)
}

// handleBundleResolved processes a successful resolve — skip confirm and start download immediately.
func (m *model) handleBundleResolved(msg *bundleResolvedMsg) (tea.Model, tea.Cmd) {
	m.bundleConfirm = bundleConfirmState{
		namespace:  msg.namespace,
		slug:       msg.slug,
		version:    msg.version,
		assetCount: msg.assetCount,
		buttonIdx:  0,
	}

	// Skip confirm screen — go directly to download/cache check.
	return m.startBundleDownload()
}

// handleBundleResolveError processes a resolve error.
func (m *model) handleBundleResolveError(msg bundleResolveErrorMsg) (tea.Model, tea.Cmd) {
	hint := "Check the bundle reference and try again"

	// If the error contains a 403 and the client is unauthenticated,
	// the bundle is likely private — guide the user to log in.
	if isForbiddenError(msg.err) && m.deps != nil && m.deps.Client != nil && !m.deps.Client.IsAuthenticated() {
		hint = "This bundle may be private. Run 'mush auth login' to authenticate"
	}

	m.bundleError = bundleErrorState{
		message:   msg.err.Error(),
		hint:      hint,
		namespace: m.bundleResolve.namespace,
		slug:      msg.slug,
		version:   msg.version,
	}

	// Replace resolving screen with error.
	m.activeScreen = screenBundleError

	return m, nil
}

// isForbiddenError returns true if the error message indicates a 403 status.
func isForbiddenError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status 403")
}

// handleBundleCacheHit processes a cache hit.
func (m *model) handleBundleCacheHit(msg bundleCacheHitMsg) (tea.Model, tea.Cmd) {
	var layers []client.BundleLayer
	if manifest, err := loadManifestFromCache(msg.cachePath); err == nil {
		layers = manifest.Manifest.Layers
	}

	m.bundleAction = bundleActionState{
		namespace: m.bundleProgress.namespace,
		slug:      m.bundleProgress.slug,
		version:   m.bundleProgress.version,
		cachePath: msg.cachePath,
		layers:    layers,
	}

	// Replace progress screen with action choice.
	m.activeScreen = screenBundleAction

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
	var layers []client.BundleLayer
	if manifest, err := loadManifestFromCache(msg.cachePath); err == nil {
		layers = manifest.Manifest.Layers
	}

	m.bundleAction = bundleActionState{
		namespace: m.bundleProgress.namespace,
		slug:      m.bundleProgress.slug,
		version:   m.bundleProgress.version,
		cachePath: msg.cachePath,
		layers:    layers,
	}

	// Replace progress screen with action choice.
	m.activeScreen = screenBundleAction

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
	}

	// Replace progress screen with error.
	m.activeScreen = screenBundleError

	return m, nil
}

// handleBundleInstallConflicts processes the install conflict check result.
func (m *model) handleBundleInstallConflicts(msg bundleInstallConflictsMsg) (tea.Model, tea.Cmd) {
	m.bundleInstallConfirm.hasConflicts = msg.hasConflicts
	m.bundleInstallConfirm.conflictPaths = msg.conflictPaths

	return m, nil
}
