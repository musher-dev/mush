package nav

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// hyperlink wraps text with OSC 8 terminal hyperlink escape sequences.
// Terminals that support OSC 8 render clickable links; others show plain text.
func hyperlink(url, text string) string {
	return ansi.SetHyperlink(url) + text + ansi.ResetHyperlink()
}

// hint pairs a key label with a short description for footer rendering.
type hint struct {
	key  string
	desc string
}

func bindingHint(binding key.Binding, desc string) hint {
	return hint{key: binding.Help().Key, desc: desc}
}

func navigationHint(up, down key.Binding, desc string) hint {
	return hint{
		key:  primaryHelpKey(up) + "/" + primaryHelpKey(down),
		desc: desc,
	}
}

func switchHint(tab, left, right key.Binding, desc string) hint {
	keys := []string{bindingHelpKey(tab)}
	if leftKey := primaryHelpKey(left); leftKey != "" {
		keys = append(keys, leftKey)
	}

	if rightKey := primaryHelpKey(right); rightKey != "" {
		keys = append(keys, rightKey)
	}

	return hint{key: strings.Join(keys, "/"), desc: desc}
}

func bindingHelpKey(binding key.Binding) string {
	return binding.Help().Key
}

// renderBorderTitle builds a top border line with an embedded title (e.g. ╭── title ────╮).
func renderBorderTitle(title string, outerWidth int, borderColor, titleStyle *lipgloss.Style) string {
	border := lipgloss.RoundedBorder()

	titleText := " " + title + " "
	titleRendered := titleStyle.Render(titleText)
	titleWidth := ansi.StringWidth(titleText)

	// Available fill = total width - left corner (1) - right corner (1).
	fillTotal := outerWidth - 2
	if fillTotal < 0 {
		fillTotal = 0
	}

	leftDashes := 2
	if leftDashes > fillTotal {
		leftDashes = fillTotal
	}

	rightDashes := fillTotal - leftDashes - titleWidth
	if rightDashes < 0 {
		rightDashes = 0
	}

	var builder strings.Builder

	builder.WriteString(borderColor.Render(border.TopLeft + strings.Repeat(border.Top, leftDashes)))
	builder.WriteString(titleRendered)
	builder.WriteString(borderColor.Render(strings.Repeat(border.Top, rightDashes) + border.TopRight))

	return builder.String()
}

// renderPanel draws a rounded-border panel with an embedded title in the top border.
func renderPanel(styles *theme, title, content string, width int, active bool) string {
	borderFg := colorBorder
	if active {
		borderFg = colorBorderAct
	}

	if title == "" {
		style := styles.panelBorder
		if active {
			style = styles.panelBorderActive
		}

		return style.Width(width).Render(content)
	}

	// Render content with border on 3 sides (no top — we build our own).
	bodyStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(borderFg).
		Padding(1, 2).
		Width(width)

	body := bodyStyle.Render(content)
	outerWidth := lipgloss.Width(body)

	borderColor := lipgloss.NewStyle().Foreground(borderFg)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(borderFg)
	if active {
		titleStyle = titleStyle.Foreground(colorAccent)
	}

	topLine := renderBorderTitle(title, outerWidth, &borderColor, &titleStyle)

	return topLine + "\n" + body
}

// renderBreadcrumb renders a trail like "Home > Load Bundle > Resolving".
func renderBreadcrumb(styles *theme, crumbs []string) string {
	sep := styles.breadcrumbSep.Render(" > ")

	var parts []string

	for _, crumb := range crumbs {
		parts = append(parts, styles.breadcrumb.Render(crumb))
	}

	return strings.Join(parts, sep)
}

// renderStatusDot renders "● label" with the given color style.
func renderStatusDot(style *lipgloss.Style, label string) string {
	return style.Render("\u25CF") + " " + style.Render(label)
}

// renderButton renders a button with focused/unfocused styling.
func renderButton(styles *theme, label string, focused bool) string {
	if focused {
		return styles.buttonActive.Render(label)
	}

	return styles.button.Render(label)
}

// renderErrorScreen renders a generic error screen with breadcrumb, message, hint, and retry/back buttons.
func renderErrorScreen(mdl *model, crumbs []string, message, hintText string, buttonIdx int) string {
	crumbLine := renderBreadcrumb(&mdl.styles, crumbs)

	retryBtn := renderButton(&mdl.styles, "Retry", buttonIdx == 0)
	backBtn := renderButton(&mdl.styles, "Back", buttonIdx == 1)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, retryBtn, "  ", backBtn)

	lines := []string{
		renderStatusDot(&mdl.styles.statusError, "Error"),
		"",
		mdl.styles.progressText.Render(message),
	}

	if hintText != "" {
		lines = append(lines, "", mdl.styles.placeholder.Render(hintText))
	}

	lines = append(lines, "", buttons)

	body := strings.Join(lines, "\n")
	panel := renderPanel(&mdl.styles, "Error", body, mdl.styles.menuWidth, true)

	footer := renderKeyHints(&mdl.styles, []hint{
		switchHint(mdl.keys.Tab, mdl.keys.Left, mdl.keys.Right, "switch"),
		bindingHint(mdl.keys.Retry, "retry"),
		bindingHint(mdl.keys.Back, "back"),
	})

	content := lipgloss.JoinVertical(lipgloss.Center, crumbLine, "", panel, "", footer)

	return lipgloss.Place(
		mdl.width, mdl.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderKeyHints renders context-sensitive footer key hints separated by dots.
func renderKeyHints(styles *theme, hints []hint) string {
	sep := styles.hintSep.Render(" \u2022 ")

	var parts []string

	for _, hintItem := range hints {
		entry := styles.hintKey.Render(hintItem.key) + " " + styles.hintDesc.Render(hintItem.desc)
		parts = append(parts, entry)
	}

	return strings.Join(parts, sep)
}
