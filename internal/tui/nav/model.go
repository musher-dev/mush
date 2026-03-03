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
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/transcript"
)

// screen identifies which screen is currently active.
type screen int

const (
	screenHome            screen = iota
	screenBundleInput            // text input for slug + harness selector
	screenBundleResolving        // spinner during API resolve
	screenBundleConfirm          // show details, confirm load
	screenBundleProgress         // download progress bar
	screenBundleComplete         // success, launch button
	screenBundleError            // error + retry/back
	screenWorkerHabitats         // habitat list (inline spinner → selection)
	screenWorkerQueues           // queue list (inline spinner → selection)
	screenWorkerHarness          // harness selector (local, no spinner)
	screenWorkerChecking         // spinner: instruction availability check
	screenWorkerConfirm          // summary + start/cancel buttons
	screenWorkerError            // error + retry/back
	screenHubExplore             // hub browse/search
	screenHubDetail              // hub bundle detail view
	screenPlaceholder            // coming-soon for unimplemented items
)

// menuItem represents a single entry in the home menu.
type menuItem struct {
	label       string
	hotkey      rune
	description string
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

// bundleInputState holds state for the bundle input screen.
type bundleInputState struct {
	textInput    textinput.Model
	harnessCur   int  // selected harness index
	focusOnInput bool // true=text input focused, false=harness list focused
}

// bundleResolveState holds state for the resolving screen.
type bundleResolveState struct {
	spinner spinner.Model
	slug    string
	version string
	cancel  context.CancelFunc
}

// bundleConfirmState holds state for the confirmation screen.
type bundleConfirmState struct {
	slug       string
	version    string
	assetCount int
	harness    string
	buttonIdx  int // 0=Load, 1=Cancel
}

// bundleProgressState holds state for the download progress screen.
type bundleProgressState struct {
	progress progress.Model
	slug     string
	version  string
	label    string
	current  int
	total    int
}

// bundleCompleteState holds state for the completion screen.
type bundleCompleteState struct {
	slug      string
	version   string
	harness   string
	cachePath string
}

// bundleErrorState holds state for the error screen.
type bundleErrorState struct {
	message string
	hint    string
	slug    string
	version string
	harness string
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

	// Harness options (from registry)
	harnesses []harnessOption

	// Context panel
	ctxInfo contextInfo

	// Bundle sub-states
	bundleInput    bundleInputState
	bundleResolve  bundleResolveState
	bundleConfirm  bundleConfirmState
	bundleProgress bundleProgressState
	bundleComplete bundleCompleteState
	bundleError    bundleErrorState

	// Hub sub-states
	hubExplore hubExploreState
	hubDetail  hubDetailState

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
	slugInput.Placeholder = "bundle-slug or slug:version"
	slugInput.CharLimit = 128
	slugInput.Width = menuWidthFull - 8 //nolint:mnd // padding

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

	hubSearchInput := textinput.New()
	hubSearchInput.Placeholder = "Search bundles..."
	hubSearchInput.CharLimit = 128
	hubSearchInput.Width = clampHubWidth(defaultWidth) - 12 //nolint:mnd // panel padding + border

	prog := progress.New(progress.WithDefaultGradient())

	return &model{
		width:     defaultWidth,
		height:    defaultHeight,
		harnesses: loadHarnesses(),
		items: []menuItem{
			{label: "Load a bundle", hotkey: 'b', description: "Install and run a skill bundle"},
			{label: "Start worker", hotkey: 'w', description: "Connect to a queue and process jobs"},
			{label: "View history", hotkey: 'h', description: "Browse recent transcript sessions"},
			{label: "Check status", hotkey: 's', description: "Run connectivity diagnostics"},
			{label: "Explore Hub", hotkey: 'e', description: "Search and browse published bundles"},
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
		bundleInput: bundleInputState{
			textInput:    slugInput,
			focusOnInput: true,
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
	}
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
		return
	}

	m.activeScreen = m.screenStack[len(m.screenStack)-1]
	m.screenStack = m.screenStack[:len(m.screenStack)-1]
}

// Init satisfies tea.Model. Fires async context loading.
func (m *model) Init() tea.Cmd {
	return cmdLoadContext(m.deps)
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

	case contextInfoMsg:
		m.ctxInfo = contextInfo{
			loading:        false,
			authStatus:     msg.authStatus,
			workspaceName:  msg.workspaceName,
			workspaceID:    msg.workspaceID,
			recentSessions: msg.recentSessions,
		}

		return m, nil

	case bundleResolvedMsg:
		return m.handleBundleResolved(msg)

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

	case workerInstructionErrorMsg:
		return m.handleWorkerInstructionError(msg)

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
	case screenBundleConfirm:
		return m.handleBundleConfirmKey(msg)
	case screenBundleProgress:
		return m.handleBundleProgressKey(msg)
	case screenBundleComplete:
		return m.handleBundleCompleteKey(msg)
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
	case screenPlaceholder:
		return m.handlePlaceholderKey(msg)
	}

	return m, nil
}

// handleHomeKey processes key events on the home screen.
func (m *model) handleHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, m.keys.Select):
		return m.activateMenuItem(m.cursor)

	default:
		// Check for hotkey match.
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			for idx, item := range m.items {
				if item.hotkey == r {
					m.cursor = idx

					return m.activateMenuItem(idx)
				}
			}
		}
	}

	return m, nil
}

// activateMenuItem handles selection of a menu item by index.
func (m *model) activateMenuItem(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.items) {
		return m, nil
	}

	switch m.items[idx].hotkey {
	case 'b':
		// Reset bundle input state.
		slugField := textinput.New()
		slugField.Placeholder = "bundle-slug or slug:version"
		slugField.CharLimit = 128
		slugField.Width = m.styles.menuWidth - 8 //nolint:mnd // padding
		slugField.Focus()

		m.bundleInput = bundleInputState{
			textInput:    slugField,
			focusOnInput: true,
		}

		m.pushScreen(screenBundleInput)

		return m, textinput.Blink

	case 'w':
		if m.deps == nil || m.deps.Client == nil {
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
			cmdListHabitats(m.deps.Client),
		)

	case 'e':
		// Reset hub explore state.
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
		}

		m.pushScreen(screenHubExplore)

		baseURL := m.apiBaseURL()

		return m, tea.Batch(
			m.hubExplore.spinner.Tick,
			cmdSearchHub(baseURL, "", "", "trending", hubSearchLimit, "", false),
			cmdListHubCategories(baseURL),
		)

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

// View renders the current screen.
func (m *model) View() string {
	switch m.activeScreen {
	case screenBundleInput:
		return renderBundleInput(m)
	case screenBundleResolving:
		return renderBundleResolving(m)
	case screenBundleConfirm:
		return renderBundleConfirm(m)
	case screenBundleProgress:
		return renderBundleProgress(m)
	case screenBundleComplete:
		return renderBundleComplete(m)
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
