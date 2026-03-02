package nav

import "github.com/charmbracelet/lipgloss"

// Responsive breakpoints.
const (
	menuWidthFull     = 42
	menuWidthCompact  = 32
	compactThreshold  = 60
	minTerminalWidth  = 40
	menuPaddingInline = 4
)

// Adaptive color palette — looks correct on both light and dark terminals.
var (
	colorAccent   = lipgloss.AdaptiveColor{Light: "#7B5EA7", Dark: "#9D7CD8"}
	colorText     = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DCDCDC"}
	colorDimText  = lipgloss.AdaptiveColor{Light: "#777777", Dark: "#888888"}
	colorMuted    = lipgloss.AdaptiveColor{Light: "#8E8E8E", Dark: "#6C6C6C"}
	colorBorder   = lipgloss.AdaptiveColor{Light: "#D0D0D0", Dark: "#444444"}
	colorActiveBG = lipgloss.AdaptiveColor{Light: "#EDE5F5", Dark: "#3A2D50"}
)

// theme holds pre-computed lipgloss styles for the current terminal width.
type theme struct {
	logo            lipgloss.Style
	version         lipgloss.Style
	tagline         lipgloss.Style
	menuItem        lipgloss.Style
	menuItemActive  lipgloss.Style
	menuBox         lipgloss.Style
	description     lipgloss.Style
	hintKey         lipgloss.Style
	hintDesc        lipgloss.Style
	hintSep         lipgloss.Style
	placeholder     lipgloss.Style
	placeholderHint lipgloss.Style
	menuWidth       int
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

// newTheme creates all styles sized for the given terminal width.
func newTheme(width int) theme {
	menuW := clampMenuWidth(width)

	return theme{
		menuWidth: menuW,

		logo: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent),

		version: lipgloss.NewStyle().
			Foreground(colorMuted),

		tagline: lipgloss.NewStyle().
			Foreground(colorDimText),

		menuItem: lipgloss.NewStyle().
			Foreground(colorText).
			Width(menuW - 6), //nolint:mnd // box width minus border (2) and horizontal padding (4)

		menuItemActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Background(colorActiveBG).
			Width(menuW - 6), //nolint:mnd // box width minus border (2) and horizontal padding (4)

		menuBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2).
			Width(menuW),

		description: lipgloss.NewStyle().
			Foreground(colorDimText).
			Width(menuW).
			Align(lipgloss.Center),

		hintKey: lipgloss.NewStyle().
			Foreground(colorAccent),

		hintDesc: lipgloss.NewStyle().
			Foreground(colorMuted),

		hintSep: lipgloss.NewStyle().
			Foreground(colorMuted),

		placeholder: lipgloss.NewStyle().
			Foreground(colorDimText).
			Italic(true),

		placeholderHint: lipgloss.NewStyle().
			Foreground(colorMuted),
	}
}
