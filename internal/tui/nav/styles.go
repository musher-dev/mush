package nav

import "github.com/charmbracelet/lipgloss"

// Responsive layout breakpoints.
const (
	twoPanelThreshold = 124 // 60 + 60 + gap(2) + margin(2)
	compactThreshold  = 60
	minTerminalWidth  = 40

	menuWidthFull     = 60
	menuWidthCompact  = 32
	hubWidthMax       = 80
	contextPanelWidth = 24

	menuPaddingInline = 4
)

// layoutMode classifies the current terminal width for responsive rendering.
type layoutMode int

const (
	layoutMinimal  layoutMode = iota // < 40 cols
	layoutCompact                    // 40–59 cols
	layoutSingle                     // 60–123 cols
	layoutTwoPanel                   // >= 124 cols
)

// classifyLayout returns the layout mode for a given terminal width.
func classifyLayout(width int) layoutMode {
	switch {
	case width >= twoPanelThreshold:
		return layoutTwoPanel
	case width >= compactThreshold:
		return layoutSingle
	case width >= minTerminalWidth:
		return layoutCompact
	default:
		return layoutMinimal
	}
}

// Blue-tinted adaptive color palette (Tokyo Night inspired).
var (
	colorAccent    = lipgloss.AdaptiveColor{Light: "#7B5EA7", Dark: "#9D7CD8"}
	colorAccentDim = lipgloss.AdaptiveColor{Light: "#9A86BE", Dark: "#7957A8"}
	colorSuccess   = lipgloss.AdaptiveColor{Light: "#3A8A55", Dark: "#9ECE6A"}
	colorWarning   = lipgloss.AdaptiveColor{Light: "#B58900", Dark: "#E0AF68"}
	colorError     = lipgloss.AdaptiveColor{Light: "#C43E3E", Dark: "#F7768E"}
	colorText      = lipgloss.AdaptiveColor{Light: "#2E3440", Dark: "#C8CEDB"}
	colorTextSec   = lipgloss.AdaptiveColor{Light: "#5A6373", Dark: "#8B95A7"}
	colorDim       = lipgloss.AdaptiveColor{Light: "#737D8C", Dark: "#636D7E"}
	colorMuted     = lipgloss.AdaptiveColor{Light: "#8E96A5", Dark: "#4E5668"}
	colorBorder    = lipgloss.AdaptiveColor{Light: "#D4D8E0", Dark: "#3B4252"}
	colorBorderAct = lipgloss.AdaptiveColor{Light: "#7B5EA7", Dark: "#9D7CD8"}
	colorSurface   = lipgloss.AdaptiveColor{Light: "#F0F1F4", Dark: "#2A2E3A"} //nolint:unused // reserved for future panel bg
	colorHighlight = lipgloss.AdaptiveColor{Light: "#E4E1F0", Dark: "#33294A"}
)

// theme holds pre-computed lipgloss styles for the current terminal width.
type theme struct {
	// Layout
	menuWidth    int
	hubWidth     int // wider panel for hub screens
	layout       layoutMode
	contextWidth int

	// Branding
	logo    lipgloss.Style
	version lipgloss.Style
	tagline lipgloss.Style

	// Menu
	menuItem       lipgloss.Style
	menuItemActive lipgloss.Style
	menuBox        lipgloss.Style
	description    lipgloss.Style
	hotkey         lipgloss.Style
	hotkeyActive   lipgloss.Style

	// Panels
	panelBorder       lipgloss.Style
	panelBorderActive lipgloss.Style

	// Section headers (menu dividers)
	sectionHeader lipgloss.Style

	// Context panel
	sectionTitle lipgloss.Style

	// Status
	statusOK      lipgloss.Style
	statusWarning lipgloss.Style
	statusError   lipgloss.Style

	// Breadcrumb
	breadcrumb    lipgloss.Style
	breadcrumbSep lipgloss.Style

	// Inputs and buttons
	inputField   lipgloss.Style
	button       lipgloss.Style
	buttonActive lipgloss.Style

	// Footer hints
	hintKey  lipgloss.Style
	hintDesc lipgloss.Style
	hintSep  lipgloss.Style

	// Placeholder
	placeholder     lipgloss.Style
	placeholderHint lipgloss.Style

	// Spinner/progress text
	spinnerText  lipgloss.Style
	progressText lipgloss.Style

	// Hub
	hubTypeBadge      lipgloss.Style
	hubStats          lipgloss.Style
	hubTrustVerified  lipgloss.Style
	hubTrustCommunity lipgloss.Style
	hubTag            lipgloss.Style
}

// clampMenuWidth picks the menu content width based on terminal width.
func clampMenuWidth(termWidth int) int {
	switch {
	case termWidth >= compactThreshold:
		return menuWidthFull
	case termWidth >= minTerminalWidth:
		return menuWidthCompact
	default:
		width := termWidth - menuPaddingInline
		if width < 20 {
			return 20
		}

		return width
	}
}

// clampHubWidth picks the hub panel width — wider than menu, capped at hubWidthMax.
func clampHubWidth(termWidth int) int {
	// Leave 4 chars margin (2 per side) for centering.
	available := termWidth - 4 //nolint:mnd // centering margin
	if available > hubWidthMax {
		available = hubWidthMax
	}

	// Never narrower than the menu width.
	menuW := clampMenuWidth(termWidth)
	if available < menuW {
		return menuW
	}

	return available
}

// newTheme creates all styles sized for the given terminal width.
func newTheme(width int) theme {
	menuW := clampMenuWidth(width)
	lay := classifyLayout(width)

	ctxW := contextPanelWidth
	if lay == layoutTwoPanel {
		ctxW = width - menuW - 5 //nolint:mnd // gap=3 + border=2
		if ctxW < contextPanelWidth {
			ctxW = contextPanelWidth
		}

		if ctxW > 36 {
			ctxW = 36
		}
	}

	hubW := clampHubWidth(width)
	itemWidth := menuW - 6 //nolint:mnd // box width minus border (2) and horizontal padding (4)

	return theme{
		menuWidth:    menuW,
		hubWidth:     hubW,
		layout:       lay,
		contextWidth: ctxW,

		logo: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent),

		version: lipgloss.NewStyle().
			Foreground(colorMuted),

		tagline: lipgloss.NewStyle().
			Foreground(colorDim),

		menuItem: lipgloss.NewStyle().
			Foreground(colorText).
			Width(itemWidth),

		menuItemActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Background(colorHighlight).
			Width(itemWidth),

		menuBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderAct).
			Padding(1, 2).
			Width(menuW),

		description: lipgloss.NewStyle().
			Foreground(colorDim).
			Width(menuW).
			Align(lipgloss.Center),

		hotkey: lipgloss.NewStyle().
			Foreground(colorAccentDim).
			Bold(true),

		hotkeyActive: lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true),

		panelBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2),

		panelBorderActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderAct).
			Padding(1, 2),

		sectionHeader: lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true),

		sectionTitle: lipgloss.NewStyle().
			Foreground(colorTextSec).
			Bold(true),

		statusOK: lipgloss.NewStyle().
			Foreground(colorSuccess),

		statusWarning: lipgloss.NewStyle().
			Foreground(colorWarning),

		statusError: lipgloss.NewStyle().
			Foreground(colorError),

		breadcrumb: lipgloss.NewStyle().
			Foreground(colorDim),

		breadcrumbSep: lipgloss.NewStyle().
			Foreground(colorMuted),

		inputField: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(menuW - 4), //nolint:mnd // inner padding

		button: lipgloss.NewStyle().
			Foreground(colorDim).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 2),

		buttonActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 2),

		hintKey: lipgloss.NewStyle().
			Foreground(colorAccent),

		hintDesc: lipgloss.NewStyle().
			Foreground(colorMuted),

		hintSep: lipgloss.NewStyle().
			Foreground(colorMuted),

		placeholder: lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true),

		placeholderHint: lipgloss.NewStyle().
			Foreground(colorMuted),

		spinnerText: lipgloss.NewStyle().
			Foreground(colorTextSec),

		progressText: lipgloss.NewStyle().
			Foreground(colorText),

		hubTypeBadge: lipgloss.NewStyle().
			Foreground(colorAccentDim).
			Bold(true),

		hubStats: lipgloss.NewStyle().
			Foreground(colorMuted),

		hubTrustVerified: lipgloss.NewStyle().
			Foreground(colorSuccess),

		hubTrustCommunity: lipgloss.NewStyle().
			Foreground(colorAccent),

		hubTag: lipgloss.NewStyle().
			Foreground(colorDim),
	}
}
