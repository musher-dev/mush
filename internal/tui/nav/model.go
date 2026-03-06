package nav

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/transcript"
	"github.com/musher-dev/mush/internal/update"
)

// screen identifies which screen is currently active.
type screen int

const (
	screenHome                 screen = iota
	screenBundleInput                 // text input for slug (no harness selector)
	screenBundleResolving             // spinner during API resolve
	screenBundleConfirm               // show details, confirm download
	screenBundleProgress              // download progress bar
	screenBundleAction                // Run / Install choice
	screenBundleHarness               // harness selection (shared by Run and Install paths)
	screenBundleInstallConfirm        // install confirmation with optional force toggle
	screenBundleError                 // error + retry/back
	screenWorkerHabitats              // habitat list (inline spinner → selection)
	screenWorkerQueues                // queue list (inline spinner → selection)
	screenWorkerHarness               // harness selector (local, no spinner)
	screenWorkerChecking              // spinner: instruction availability check
	screenWorkerConfirm               // summary + start/cancel buttons
	screenWorkerError                 // error + retry/back
	screenHubExplore                  // hub browse/search
	screenHubDetail                   // hub bundle detail view
	screenStatus                      // connectivity diagnostics
	screenHistory                     // transcript session list
	screenHistoryDetail               // transcript session detail viewer
	screenPlaceholder                 // coming-soon for unimplemented items
)

// menuItem represents a single entry in the home menu.
type menuItem struct {
	label       string
	hotkey      rune
	description string
	isSection   bool // section headers are non-selectable dividers
}

// harnessOption is one of the available harness choices.
type harnessOption struct {
	name string
	desc string
}

func loadHarnesses() []harnessOption {
	var opts []harnessOption

	for _, name := range harness.ProviderNames() {
		spec, ok := harness.GetProvider(name)
		if !ok {
			continue
		}

		opts = append(opts, harnessOption{name: spec.Name, desc: spec.Description})
	}

	return opts
}

// bundleInputFocus identifies which area of the bundle input screen has focus.
type bundleInputFocus int

const (
	bundleFocusInput bundleInputFocus = iota // text input
	bundleFocusList                          // recent/installed/action list
)

// bundleInputState holds state for the bundle input screen.
type bundleInputState struct {
	textInput textinput.Model
	focusArea bundleInputFocus // which area is focused

	// Enriched bundle selection.
	recentBundles    []recentBundleEntry
	installedBundles []installedBundleEntry
	listCursor       int // cursor in the combined recent+installed+links list
	listLoaded       bool
}

// recentBundleEntry is a recently cached bundle for the Run harness screen.
type recentBundleEntry struct {
	namespace string
	slug      string
	version   string
	timeAgo   string
}

// installedBundleEntry is an installed bundle for the Run harness screen.
type installedBundleEntry struct {
	namespace string
	slug      string
	ref       string
	version   string
	harness   string
}

// bundleResolveState holds state for the resolving screen.
type bundleResolveState struct {
	spinner   spinner.Model
	namespace string
	slug      string
	version   string
	cancel    context.CancelFunc
}

// bundleConfirmState holds state for the confirmation screen.
type bundleConfirmState struct {
	namespace  string
	slug       string
	version    string
	assetCount int
	buttonIdx  int // 0=Download, 1=Cancel
}

// bundleProgressState holds state for the download progress screen.
type bundleProgressState struct {
	progress  progress.Model
	namespace string
	slug      string
	version   string
	label     string
	current   int
	total     int
}

// bundleActionState holds state for the Run/Install action choice screen.
type bundleActionState struct {
	namespace string
	slug      string
	version   string
	cachePath string
	buttonIdx int // 0=Run, 1=Install
}

// bundleHarnessState holds state for the harness selection screen (shared by Run and Install).
type bundleHarnessState struct {
	namespace  string
	slug       string
	version    string
	cachePath  string
	cursor     int   // selected index within installed list
	installed  []int // indices into model.harnesses for installed harnesses
	forInstall bool  // true=Install path, false=Run path
}

// bundleInstallConfirmState holds state for the install confirmation screen.
type bundleInstallConfirmState struct {
	namespace     string
	slug          string
	version       string
	cachePath     string
	harness       string
	hasConflicts  bool     // true if existing files detected
	force         bool     // overwrite toggle (only shown if hasConflicts)
	buttonIdx     int      // 0=Install, 1=Cancel
	targetDir     string   // CWD
	conflictPaths []string // which files conflict
}

// bundleErrorState holds state for the error screen.
type bundleErrorState struct {
	message   string
	hint      string
	namespace string
	slug      string
	version   string
	buttonIdx int // 0=Retry, 1=Back
}

// workerHabitatsState holds state for the habitat selection screen.
type workerHabitatsState struct {
	spinner  spinner.Model
	loading  bool
	habitats []client.HabitatSummary
	cursor   int
}

// workerQueuesState holds state for the queue selection screen.
type workerQueuesState struct {
	spinner     spinner.Model
	loading     bool
	queues      []client.QueueSummary
	cursor      int
	habitatID   string
	habitatName string
}

// workerHarnessState holds state for the harness selection screen.
type workerHarnessState struct {
	cursor      int
	habitatID   string
	habitatName string
	queueID     string
	queueName   string
}

// workerCheckingState holds state for the instruction check spinner screen.
type workerCheckingState struct {
	spinner     spinner.Model
	habitatID   string
	habitatName string
	queueID     string
	queueName   string
	harness     string
}

// workerConfirmState holds state for the start confirmation screen.
type workerConfirmState struct {
	habitatID   string
	habitatName string
	queueID     string
	queueName   string
	harness     string
	buttonIdx   int // 0=Start, 1=Cancel
}

// workerErrorState holds state for the worker error screen.
type workerErrorState struct {
	message     string
	hint        string
	retryAction string // "habitats", "queues", "instructions"
	// Carry forward selections for retry.
	habitatID   string
	habitatName string
	queueID     string
	queueName   string
	harness     string
	buttonIdx   int // 0=Retry, 1=Back
}

// hubExploreState holds state for the hub explore screen.
type hubExploreState struct {
	searchInput  textinput.Model
	categories   []client.HubCategory
	categoryCur  int // -1=All, 0..N for categories
	loading      bool
	spinner      spinner.Model
	query        string // committed search query
	pendingQuery string // query awaiting debounce
	debounceID   int    // monotonic counter for stale-tick invalidation
	searchID     int    // monotonic counter to discard out-of-order results
	nextCursor   string
	hasMore      bool
	results      []client.HubBundleSummary
	resultCur    int // cursor in results list
	focusArea    int // 0=search, 1=categories, 2=list
	errorMsg     string
}

// hubDetailState holds state for the hub detail screen.
type hubDetailState struct {
	loading      bool
	spinner      spinner.Model
	detail       *client.HubBundleDetail
	publisher    string
	slug         string
	scrollOffset int
	errorMsg     string
}

// statusState holds state for the diagnostics status screen.
type statusState struct {
	spinner        spinner.Model
	loading        bool
	results        []doctor.Result
	passed         int
	failed         int
	warnings       int
	harnessLoading bool
	harnessReports []*harness.HealthReport
}

// historyListState holds state for the history session list screen.
type historyListState struct {
	spinner  spinner.Model
	loading  bool
	sessions []transcript.Session
	cursor   int
	errorMsg string
}

// historyDetailState holds state for the history detail viewer screen.
type historyDetailState struct {
	spinner      spinner.Model
	loading      bool
	session      transcript.Session
	events       []transcript.Event
	lines        []string // ANSI-stripped display lines
	scrollOffset int
	errorMsg     string
}

// homeHarnessState holds state for the harness sidebar panel on the home screen.
type homeHarnessState struct {
	cursor   int                  // selected harness index
	expanded int                  // expanded harness index (-1 if none)
	statuses []harnessQuickStatus // quick status for each harness
	loading  bool                 // true while initial statuses are loading
}

// harnessQuickStatus holds the quick status for a single harness on the home panel.
type harnessQuickStatus struct {
	name        string // provider name (e.g. "claude")
	displayName string // display name (e.g. "Claude Code")
	installed   bool
	version     string // version string if installed, empty otherwise
}

// harnessExpandState holds state for an expanded harness row in the home panel.
type harnessExpandState struct {
	loading bool
	spinner spinner.Model
	report  *harness.HealthReport
}

// contextInfo holds async-loaded context data for the sidebar panel.
type contextInfo struct {
	loading        bool
	authStatus     string // "authenticated", "not authenticated"
	workspaceName  string
	workspaceID    string
	recentSessions []transcript.Session
}

// model is the top-level Bubbletea model for the interactive TUI.
type model struct {
	width           int
	height          int
	cursor          int
	items           []menuItem
	activeScreen    screen
	screenStack     []screen
	placeholderText string
	keys            keyMap
	styles          theme
	deps            *Dependencies
	ctx             context.Context
	result          *Result

	// Update availability
	updateAvailable bool
	updateVersion   string

	// Harness options (from registry)
	harnesses []harnessOption

	// Home harness panel
	homeHarness   homeHarnessState
	homeFocusArea int // 0=menu, 1=harness panel
	harnessExpand harnessExpandState

	// Context panel
	ctxInfo contextInfo

	// Bundle sub-states
	bundleInput          bundleInputState
	bundleResolve        bundleResolveState
	bundleConfirm        bundleConfirmState
	bundleProgress       bundleProgressState
	bundleAction         bundleActionState
	bundleHarness        bundleHarnessState
	bundleInstallConfirm bundleInstallConfirmState
	bundleError          bundleErrorState

	// Hub sub-states
	hubExplore hubExploreState
	hubDetail  hubDetailState

	// Status sub-state
	status statusState

	// History sub-states
	history       historyListState
	historyDetail historyDetailState

	// Worker sub-states
	workerHabitats workerHabitatsState
	workerQueues   workerQueuesState
	workerHarness  workerHarnessState
	workerChecking workerCheckingState
	workerConfirm  workerConfirmState
	workerError    workerErrorState
}

// defaultWidth is the assumed terminal width before the first WindowSizeMsg.
const defaultWidth = 80

// defaultHeight is the assumed terminal height before the first WindowSizeMsg.
const defaultHeight = 24

func newModel(ctx context.Context, deps *Dependencies) *model {
	slugInput := textinput.New()
	slugInput.Placeholder = "namespace/slug or namespace/slug:version"
	slugInput.CharLimit = 128
	slugInput.Width = menuWidthFull - 8 //nolint:mnd // padding

	harnessExpandSpinner := spinner.New()
	harnessExpandSpinner.Spinner = spinner.Dot

	resolveSpinner := spinner.New()
	resolveSpinner.Spinner = spinner.Dot

	habitatsSpinner := spinner.New()
	habitatsSpinner.Spinner = spinner.Dot

	queuesSpinner := spinner.New()
	queuesSpinner.Spinner = spinner.Dot

	checkingSpinner := spinner.New()
	checkingSpinner.Spinner = spinner.Dot

	hubExploreSpinner := spinner.New()
	hubExploreSpinner.Spinner = spinner.Dot

	hubDetailSpinner := spinner.New()
	hubDetailSpinner.Spinner = spinner.Dot

	statusSpinner := spinner.New()
	statusSpinner.Spinner = spinner.Dot

	historySpinner := spinner.New()
	historySpinner.Spinner = spinner.Dot

	historyDetailSpinner := spinner.New()
	historyDetailSpinner.Spinner = spinner.Dot

	hubSearchInput := textinput.New()
	hubSearchInput.Placeholder = "Search bundles..."
	hubSearchInput.CharLimit = 128
	hubSearchInput.Width = clampHubWidth(defaultWidth) - 12 //nolint:mnd // panel padding + border

	prog := progress.New(progress.WithDefaultGradient())

	mdl := &model{
		width:     defaultWidth,
		height:    defaultHeight,
		harnesses: loadHarnesses(),
		homeHarness: homeHarnessState{
			expanded: -1,
			loading:  true,
		},
		harnessExpand: harnessExpandState{
			spinner: harnessExpandSpinner,
		},
		cursor: 1, // skip first section header
		items: []menuItem{
			{label: "DEVELOP", isSection: true},
			{label: "Load bundle", hotkey: 'r', description: "Load a bundle to run or install"},
			{label: "Find a bundle", hotkey: 'f', description: "Browse and install bundles from the Hub"},
			{label: "OPERATE", isSection: true},
			{label: "Start runner", hotkey: 'w', description: "Connect to a queue and process jobs"},
			{label: "View history", hotkey: 'h', description: "Browse recent transcript sessions"},
		},
		activeScreen: screenHome,
		screenStack:  nil,
		keys:         defaultKeyMap(),
		styles:       newTheme(defaultWidth),
		deps:         deps,
		ctx:          ctx,
		ctxInfo:      contextInfo{loading: true},
		hubExplore: hubExploreState{
			searchInput: hubSearchInput,
			categoryCur: -1,
			spinner:     hubExploreSpinner,
		},
		hubDetail: hubDetailState{
			spinner: hubDetailSpinner,
		},
		status: statusState{
			spinner: statusSpinner,
		},
		bundleInput: bundleInputState{
			textInput: slugInput,
			focusArea: bundleFocusInput,
		},
		bundleResolve: bundleResolveState{
			spinner: resolveSpinner,
		},
		bundleProgress: bundleProgressState{
			progress: prog,
		},
		workerHabitats: workerHabitatsState{
			spinner: habitatsSpinner,
		},
		workerQueues: workerQueuesState{
			spinner: queuesSpinner,
		},
		workerChecking: workerCheckingState{
			spinner: checkingSpinner,
		},
		history: historyListState{
			spinner: historySpinner,
		},
		historyDetail: historyDetailState{
			spinner: historyDetailSpinner,
		},
	}

	// If a bundle seed is provided, start directly at the action screen.
	if deps != nil && deps.InitialBundle != nil {
		mdl.bundleAction = bundleActionState{
			namespace: deps.InitialBundle.Namespace,
			slug:      deps.InitialBundle.Slug,
			version:   deps.InitialBundle.Version,
			cachePath: deps.InitialBundle.CachePath,
		}
		mdl.activeScreen = screenBundleAction
		mdl.screenStack = []screen{screenHome}
	}

	return mdl
}

// pushScreen pushes the current screen onto the stack and switches to the target.
func (m *model) pushScreen(s screen) {
	m.screenStack = append(m.screenStack, m.activeScreen)
	m.activeScreen = s
}

// popScreen returns to the previous screen on the stack.
func (m *model) popScreen() {
	if len(m.screenStack) == 0 {
		m.activeScreen = screenHome
	} else {
		m.activeScreen = m.screenStack[len(m.screenStack)-1]
		m.screenStack = m.screenStack[:len(m.screenStack)-1]
	}

	// Reset harness panel focus/expansion when returning to home.
	if m.activeScreen == screenHome {
		m.homeFocusArea = 0
		m.homeHarness.expanded = -1
		m.harnessExpand.loading = false
		m.harnessExpand.report = nil
	}
}

// updateCheckMsg carries the result of a background update check.
type updateCheckMsg struct {
	available bool
	version   string
}

// cmdCheckUpdate checks for available updates asynchronously.
func cmdCheckUpdate(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if update.IsDisabled() || buildinfo.Version == "dev" {
			return updateCheckMsg{}
		}

		state, err := update.LoadState()
		if err != nil {
			return updateCheckMsg{}
		}

		// If cache is stale, refresh it in the background.
		if state.ShouldCheck() {
			updater, err := update.NewUpdater()
			if err == nil {
				info, err := updater.CheckLatest(ctx, buildinfo.Version)
				if err == nil {
					state = &update.State{
						LastCheckedAt:  state.LastCheckedAt,
						LatestVersion:  info.LatestVersion,
						CurrentVersion: buildinfo.Version,
						ReleaseURL:     info.ReleaseURL,
					}
					_ = update.SaveState(state)
				}
			}
		}

		if state.HasUpdate(buildinfo.Version) {
			return updateCheckMsg{available: true, version: state.LatestVersion}
		}

		return updateCheckMsg{}
	}
}

// Init satisfies tea.Model. Fires async context loading and harness status detection.
func (m *model) Init() tea.Cmd {
	return tea.Batch(cmdLoadContext(m.ctx, m.deps), cmdLoadHarnessStatuses(m.ctx), cmdCheckUpdate(m.ctx))
}

// Update handles messages and returns the updated model.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.styles = newTheme(msg.Width)

		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case updateCheckMsg:
		m.updateAvailable = msg.available
		m.updateVersion = msg.version

		return m, nil

	case contextInfoMsg:
		m.ctxInfo = contextInfo{
			loading:        false,
			authStatus:     msg.authStatus,
			workspaceName:  msg.workspaceName,
			workspaceID:    msg.workspaceID,
			recentSessions: msg.recentSessions,
		}

		return m, nil

	case bundleListLoadedMsg:
		return m.handleBundleListLoaded(msg)

	case bundleResolvedMsg:
		return m.handleBundleResolved(&msg)

	case bundleResolveErrorMsg:
		return m.handleBundleResolveError(msg)

	case bundleCacheHitMsg:
		return m.handleBundleCacheHit(msg)

	case bundleDownloadProgressMsg:
		return m.handleBundleDownloadProgress(msg)

	case bundleDownloadCompleteMsg:
		return m.handleBundleDownloadComplete(msg)

	case bundleDownloadErrorMsg:
		return m.handleBundleDownloadError(msg)

	case bundleInstallConflictsMsg:
		return m.handleBundleInstallConflicts(msg)

	case workerHabitatsLoadedMsg:
		return m.handleWorkerHabitatsLoaded(msg)

	case workerHabitatsErrorMsg:
		return m.handleWorkerHabitatsError(msg)

	case workerQueuesLoadedMsg:
		return m.handleWorkerQueuesLoaded(msg)

	case workerQueuesErrorMsg:
		return m.handleWorkerQueuesError(msg)

	case workerInstructionCheckMsg:
		return m.handleWorkerInstructionCheck(msg)

	case hubSearchResultMsg:
		return m.handleHubSearchResult(msg)

	case hubSearchErrorMsg:
		return m.handleHubSearchError(msg)

	case hubDetailLoadedMsg:
		return m.handleHubDetailLoaded(msg)

	case hubDetailErrorMsg:
		return m.handleHubDetailError(msg)

	case hubCategoriesLoadedMsg:
		return m.handleHubCategoriesLoaded(msg)

	case hubDebounceTickMsg:
		return m.handleHubDebounceTick(msg)

	case historySessionsLoadedMsg:
		return m.handleHistorySessionsLoaded(msg)

	case historyEventsLoadedMsg:
		return m.handleHistoryEventsLoaded(&msg)

	case statusChecksCompleteMsg:
		return m.handleStatusChecksComplete(msg)

	case harnessHealthCompleteMsg:
		return m.handleHarnessHealthComplete(msg)

	case workerInstructionErrorMsg:
		return m.handleWorkerInstructionError(msg)

	case harnessStatusesLoadedMsg:
		return m.handleHarnessStatusesLoaded(msg)

	case harnessExpandHealthMsg:
		return m.handleHarnessExpandHealth(msg)

	case spinner.TickMsg:
		if m.activeScreen == screenBundleResolving {
			var cmd tea.Cmd

			m.bundleResolve.spinner, cmd = m.bundleResolve.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenWorkerHabitats && m.workerHabitats.loading {
			var cmd tea.Cmd

			m.workerHabitats.spinner, cmd = m.workerHabitats.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenWorkerQueues && m.workerQueues.loading {
			var cmd tea.Cmd

			m.workerQueues.spinner, cmd = m.workerQueues.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenHubExplore && m.hubExplore.loading {
			var cmd tea.Cmd

			m.hubExplore.spinner, cmd = m.hubExplore.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenHubDetail && m.hubDetail.loading {
			var cmd tea.Cmd

			m.hubDetail.spinner, cmd = m.hubDetail.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenWorkerChecking {
			var cmd tea.Cmd

			m.workerChecking.spinner, cmd = m.workerChecking.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenStatus && (m.status.loading || m.status.harnessLoading) {
			var cmd tea.Cmd

			m.status.spinner, cmd = m.status.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenHistory && m.history.loading {
			var cmd tea.Cmd

			m.history.spinner, cmd = m.history.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenHistoryDetail && m.historyDetail.loading {
			var cmd tea.Cmd

			m.historyDetail.spinner, cmd = m.historyDetail.spinner.Update(msg)

			return m, cmd
		}

		if m.activeScreen == screenHome && m.harnessExpand.loading {
			var cmd tea.Cmd

			m.harnessExpand.spinner, cmd = m.harnessExpand.spinner.Update(msg)

			return m, cmd
		}

	case progress.FrameMsg:
		if m.activeScreen == screenBundleProgress {
			mdl, cmd := m.bundleProgress.progress.Update(msg)

			if progressModel, ok := mdl.(progress.Model); ok {
				m.bundleProgress.progress = progressModel
			}

			return m, cmd
		}
	}

	return m, nil
}

// handleKey dispatches key events to the active screen handler.
func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit bindings work on every screen.
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}

	switch m.activeScreen {
	case screenHome:
		return m.handleHomeKey(msg)
	case screenBundleInput:
		return m.handleBundleInputKey(msg)
	case screenBundleResolving:
		return m.handleBundleResolvingKey(msg)
	case screenBundleProgress:
		return m.handleBundleProgressKey(msg)
	case screenBundleAction:
		return m.handleBundleActionKey(msg)
	case screenBundleHarness:
		return m.handleBundleHarnessKey(msg)
	case screenBundleInstallConfirm:
		return m.handleBundleInstallConfirmKey(msg)
	case screenBundleError:
		return m.handleBundleErrorKey(msg)
	case screenWorkerHabitats:
		return m.handleWorkerHabitatsKey(msg)
	case screenWorkerQueues:
		return m.handleWorkerQueuesKey(msg)
	case screenWorkerHarness:
		return m.handleWorkerHarnessKey(msg)
	case screenWorkerChecking:
		return m.handleWorkerCheckingKey(msg)
	case screenWorkerConfirm:
		return m.handleWorkerConfirmKey(msg)
	case screenWorkerError:
		return m.handleWorkerErrorKey(msg)
	case screenHubExplore:
		return m.handleHubExploreKey(msg)
	case screenHubDetail:
		return m.handleHubDetailKey(msg)
	case screenStatus:
		return m.handleStatusKey(msg)
	case screenHistory:
		return m.handleHistoryListKey(msg)
	case screenHistoryDetail:
		return m.handleHistoryDetailKey(msg)
	case screenPlaceholder:
		return m.handlePlaceholderKey(msg)
	}

	return m, nil
}

// handleHomeKey processes key events on the home screen.
func (m *model) handleHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Menu hotkeys work regardless of focus area.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]

		// Hidden comma hotkey for settings/status screen.
		if r == ',' {
			m.status = statusState{
				spinner:        m.status.spinner,
				loading:        true,
				harnessLoading: true,
			}

			m.pushScreen(screenStatus)

			return m, tea.Batch(m.status.spinner.Tick, cmdRunStatusChecks(m.ctx), cmdRunHarnessHealthChecks(m.ctx))
		}

		for idx, item := range m.items {
			if item.hotkey == r {
				m.cursor = idx

				return m.activateMenuItem(idx)
			}
		}
	}

	// Tab toggles focus between menu and harness panel (only in two-panel mode with harnesses).
	if key.Matches(msg, m.keys.Tab) && m.styles.layout == layoutTwoPanel && len(m.homeHarness.statuses) > 0 {
		m.homeFocusArea = (m.homeFocusArea + 1) % 2 //nolint:mnd // toggle between 2 areas

		// Collapse any expansion when switching away from harness panel.
		if m.homeFocusArea == 0 {
			m.homeHarness.expanded = -1
			m.harnessExpand.loading = false
			m.harnessExpand.report = nil
		}

		return m, nil
	}

	// Delegate to harness panel handler when focused.
	if m.homeFocusArea == 1 {
		return m.handleHarnessPanelKey(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Down):
		next := m.cursor + 1
		for next < len(m.items) && m.items[next].isSection {
			next++
		}

		if next < len(m.items) {
			m.cursor = next
		}

	case key.Matches(msg, m.keys.Up):
		prev := m.cursor - 1
		for prev >= 0 && m.items[prev].isSection {
			prev--
		}

		if prev >= 0 {
			m.cursor = prev
		}

	case key.Matches(msg, m.keys.Select):
		return m.activateMenuItem(m.cursor)
	}

	return m, nil
}

// activateMenuItem handles selection of a menu item by index.
func (m *model) activateMenuItem(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.items) {
		return m, nil
	}

	switch m.items[idx].hotkey {
	case 'r':
		// Reset bundle input state ("Run harness").
		slugField := textinput.New()
		slugField.Placeholder = "namespace/slug or namespace/slug:version"
		slugField.CharLimit = 128
		slugField.Width = m.styles.menuWidth - 8 //nolint:mnd // padding
		slugField.Focus()

		m.bundleInput = bundleInputState{
			textInput: slugField,
			focusArea: bundleFocusInput,
		}

		m.pushScreen(screenBundleInput)

		workDir := ""
		if m.deps != nil {
			workDir = m.deps.WorkDir
		}

		return m, tea.Batch(textinput.Blink, cmdLoadBundleLists(workDir))

	case 'w':
		if m.deps == nil || m.deps.Client == nil || !m.deps.Client.IsAuthenticated() {
			m.workerError = workerErrorState{
				message:     "Not authenticated",
				hint:        "Run 'mush auth login' first to authenticate",
				retryAction: "habitats",
			}

			m.pushScreen(screenWorkerError)

			return m, nil
		}

		m.workerHabitats = workerHabitatsState{
			spinner: m.workerHabitats.spinner,
			loading: true,
		}

		m.pushScreen(screenWorkerHabitats)

		return m, tea.Batch(
			m.workerHabitats.spinner.Tick,
			cmdListHabitats(m.ctx, m.deps.Client),
		)

	case 'f':
		// Reset hub explore state ("Find a bundle").
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
			cmdSearchHub(m.ctx, baseURL, "", "", "trending", hubSearchLimit, "", false, m.hubExplore.searchID),
			cmdListHubCategories(m.ctx, baseURL),
		)

	case 'h':
		m.history = historyListState{
			spinner: m.history.spinner,
			loading: true,
		}

		m.pushScreen(screenHistory)

		return m, tea.Batch(m.history.spinner.Tick, cmdLoadHistorySessions())

	default:
		m.placeholderText = m.items[idx].label
		m.pushScreen(screenPlaceholder)
	}

	return m, nil
}

// handlePlaceholderKey processes key events on the placeholder screen.
func (m *model) handlePlaceholderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Select) {
		m.popScreen()
	}

	return m, nil
}

// handleErrorScreenKey is the shared key handler for error screens with
// Retry / Back buttons. It manages button toggling, back navigation, and
// retry dispatch. Both worker and bundle error screens delegate here.
func (m *model) handleErrorScreenKey(
	msg tea.KeyMsg,
	buttonIdx *int,
	retryFn func() (tea.Model, tea.Cmd),
) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.popScreen()

	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Right):
		*buttonIdx = (*buttonIdx + 1) % 2 //nolint:mnd // 2 buttons

	case key.Matches(msg, m.keys.Retry):
		return retryFn()

	case key.Matches(msg, m.keys.Select):
		if *buttonIdx == 1 {
			m.popScreen()

			return m, nil
		}

		return retryFn()
	}

	return m, nil
}

// View renders the current screen.
func (m *model) View() string {
	switch m.activeScreen {
	case screenBundleInput:
		return renderBundleInput(m)
	case screenBundleResolving:
		return renderBundleResolving(m)
	case screenBundleProgress:
		return renderBundleProgress(m)
	case screenBundleAction:
		return renderBundleAction(m)
	case screenBundleHarness:
		return renderBundleHarness(m)
	case screenBundleInstallConfirm:
		return renderBundleInstallConfirm(m)
	case screenBundleError:
		return renderBundleError(m)
	case screenWorkerHabitats:
		return renderWorkerHabitats(m)
	case screenWorkerQueues:
		return renderWorkerQueues(m)
	case screenWorkerHarness:
		return renderWorkerHarness(m)
	case screenWorkerChecking:
		return renderWorkerChecking(m)
	case screenWorkerConfirm:
		return renderWorkerConfirm(m)
	case screenWorkerError:
		return renderWorkerError(m)
	case screenHubExplore:
		return renderHubExplore(m)
	case screenHubDetail:
		return renderHubDetail(m)
	case screenStatus:
		return renderStatus(m)
	case screenHistory:
		return renderHistoryList(m)
	case screenHistoryDetail:
		return renderHistoryDetail(m)
	case screenPlaceholder:
		return renderPlaceholder(m)
	default:
		return renderHome(m)
	}
}

// version returns the build version string for display.
func (m *model) version() string {
	return buildinfo.Version
}
