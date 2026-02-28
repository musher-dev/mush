package status

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/musher-dev/mush/internal/harness/state"
	"github.com/musher-dev/mush/internal/tui/ansi"
	"github.com/musher-dev/mush/internal/tui/render"
)

// Render produces the ANSI-escaped status bar and sidebar output.
func Render(s *state.Snapshot) string {
	var b strings.Builder
	b.WriteString(ansi.SaveCursor)

	b.WriteString(ansi.Move(1, 1))
	b.WriteString(ansi.ClearLine)
	b.WriteString(topBarLine(s))

	if s.SidebarVisible {
		rows := s.Height - 1
		lines := sidebarLines(s, rows)

		for i := 0; i < rows; i++ {
			b.WriteString(ansi.Move(2+i, 1))
			b.WriteString(sidebarRow(lines[i], s.SidebarWidth))
		}
	}

	b.WriteString(ansi.RestoreCursor)

	return b.String()
}

func topBarLine(s *state.Snapshot) string {
	parts := []string{
		"\x1b[1mMUSH\x1b[0m",
		fmt.Sprintf("Status: %s", styleStatus(s.StatusLabel)),
	}
	if s.CopyMode {
		parts = append(parts, "Mode: \x1b[33mCOPY\x1b[0m")
	} else {
		parts = append(parts, "Mode: \x1b[32mLIVE\x1b[0m")
	}

	line := strings.Join(parts, " \x1b[90m|\x1b[0m ")
	line = "\x1b[48;5;236m\x1b[38;5;252m " + line
	line = render.PadRightVisible(line, s.Width-1)

	return line + " \x1b[0m"
}

func sidebarLines(s *state.Snapshot, rows int) []string {
	lines := make([]string, 0, rows)

	lines = append(lines, "Bundle")
	if s.BundleName == "" {
		lines = append(lines, "  none loaded")
	} else {
		bundle := s.BundleName
		if s.BundleVer != "" {
			bundle += " v" + s.BundleVer
		}

		lines = append(lines,
			"  "+bundle,
			fmt.Sprintf("  layers: %d", s.BundleLayers),
			fmt.Sprintf("  agents: %d", len(s.BundleAgents)),
			fmt.Sprintf("  skills: %d", len(s.BundleSkills)),
			fmt.Sprintf("  tools: %d", len(s.BundleTools)),
		)

		if len(s.BundleOther) > 0 {
			lines = append(lines, fmt.Sprintf("  other: %d", len(s.BundleOther)))
		}
	}

	appendList := func(title string, names []string) {
		if len(names) == 0 {
			return
		}

		lines = append(lines, "", title)

		maxItems := 4
		for i := 0; i < len(names) && i < maxItems; i++ {
			lines = append(lines, "  - "+names[i])
		}

		if len(names) > maxItems {
			lines = append(lines, fmt.Sprintf("  +%d more", len(names)-maxItems))
		}
	}

	agents := append([]string(nil), s.BundleAgents...)
	skills := append([]string(nil), s.BundleSkills...)
	tools := append([]string(nil), s.BundleTools...)

	sort.Strings(agents)
	sort.Strings(skills)
	sort.Strings(tools)
	appendList("Agents", agents)
	appendList("Skills", skills)
	appendList("Tools", tools)

	lines = append(lines, "", "MCP")
	if len(s.MCPServers) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, server := range s.MCPServers {
			flags := []string{}

			if server.Loaded {
				flags = append(flags, "loaded")
			} else {
				flags = append(flags, "off")
			}

			switch {
			case server.Authenticated:
				flags = append(flags, "auth")
			case server.Expired:
				flags = append(flags, "expired")
			default:
				flags = append(flags, "no-auth")
			}

			lines = append(lines, fmt.Sprintf("  %s (%s)", server.Name, strings.Join(flags, ",")))
		}
	}

	lines = append(lines, "", "Session")

	if s.QueueID != "" {
		lines = append(lines, "  queue: "+s.QueueID)
	}

	if len(s.SupportedHarnesses) > 0 {
		lines = append(lines, "  harness: "+strings.Join(s.SupportedHarnesses, ", "))
	}

	if s.LastError != "" && s.Now.Sub(s.LastErrorTime) < 30*time.Second {
		msg := s.LastError
		if utf8.RuneCountInString(msg) > 30 {
			msg = truncatePlain(msg, 27) + "..."
		}

		lines = append(lines, "  err: "+msg)
	}

	for len(lines) < rows {
		lines = append(lines, "")
	}

	if len(lines) > rows {
		lines = lines[:rows]
	}

	return lines
}

func sidebarRow(content string, sidebarWidth int) string {
	maxContent := sidebarWidth - 2
	if maxContent < 0 {
		maxContent = 0
	}

	content = truncatePlain(content, maxContent)

	body := " " + content
	body = render.PadRightVisible(body, sidebarWidth)

	return "\x1b[48;5;238m\x1b[38;5;252m" + body + "\x1b[48;5;236m\x1b[38;5;244m│\x1b[0m"
}

func truncatePlain(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	out := make([]rune, 0, maxRunes)

	for _, r := range s {
		if len(out) >= maxRunes {
			break
		}

		out = append(out, r)
	}

	return string(out)
}

func styleStatus(label string) string {
	switch label {
	case "Starting...":
		return "\x1b[33m\x1b[1mStarting\x1b[0m"
	case "Ready":
		return "\x1b[32m\x1b[1mReady\x1b[0m"
	case "Connected":
		return "\x1b[32m\x1b[1mConnected\x1b[0m"
	case "Processing":
		return "\x1b[33m\x1b[1mProcessing\x1b[0m"
	case "Error":
		return "\x1b[31m\x1b[1mError\x1b[0m"
	default:
		return label
	}
}
