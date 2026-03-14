package status

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

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
		lines, _ := SidebarLines(s, rows)

		for i := 0; i < rows; i++ {
			b.WriteString(ansi.Move(2+i, 1))
			b.WriteString(sidebarRow(lines[i], s.SidebarWidth))
		}
	}

	b.WriteString(ansi.RestoreCursor)

	return b.String()
}

const (
	// Status bar palette.
	barBG    = "\x1b[48;5;236m"              // bar background
	barFG    = "\x1b[38;5;252m"              // bar foreground
	barReset = "\x1b[22;39m" + barBG + barFG // clear bold, reset FG, re-apply BG+FG

	// Accent colors (applied over bar background).
	bold     = "\x1b[1m"
	dimGray  = "\x1b[90m"
	green    = "\x1b[38;5;149m" // matches colorSuccess (#9ECE6A)
	yellow   = "\x1b[38;5;179m" // matches colorWarning (#E0AF68)
	red      = "\x1b[38;5;210m" // matches colorError (#F7768E)
	accentFG = "\x1b[38;5;140m" // matches colorAccent (#9D7CD8)

	// Sidebar palette.
	sidebarBG     = "\x1b[48;5;238m"
	sidebarFG     = "\x1b[38;5;252m"
	sidebarBorder = "\x1b[48;5;236m\x1b[38;5;240m" // muted border

	// Reset.
	resetAll = "\x1b[0m"
)

func topBarLine(s *state.Snapshot) string {
	sep := " " + dimGray + "|" + barReset + " "

	parts := []string{
		accentFG + bold + "MUSH" + barReset,
		fmt.Sprintf("Status: %s", styleStatus(s.StatusLabel)),
	}
	if s.CopyMode {
		parts = append(parts, "Mode: "+yellow+"COPY"+barReset)
	} else {
		parts = append(parts, "Mode: "+green+"LIVE"+barReset)
	}

	// Keyboard shortcut hints — always visible for discoverability.
	parts = append(parts, dimGray+"^C Int  ^S Copy  ^Q Quit"+barReset)

	line := strings.Join(parts, sep)
	line = barBG + barFG + " " + line
	line = render.PadRightVisible(line, s.Width-1)

	return line + " " + resetAll
}

// SidebarClickTarget identifies a clickable row in the sidebar.
type SidebarClickTarget struct {
	Row     int    // 0-based index into returned lines
	Section string // "Agents", "Skills", or "Tools"
}

// SidebarLines builds plain-text sidebar rows for a snapshot.
// It dynamically sizes lists based on the available rows and returns
// click targets for expandable/collapsible list sections.
func SidebarLines(s *state.Snapshot, rows int) ([]string, []SidebarClickTarget) {
	lines := make([]string, 0, rows)

	var targets []SidebarClickTarget

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

	agents := append([]string(nil), s.BundleAgents...)
	skills := append([]string(nil), s.BundleSkills...)
	tools := append([]string(nil), s.BundleTools...)

	sort.Strings(agents)
	sort.Strings(skills)
	sort.Strings(tools)

	// Count fixed lines for MCP and Interaction sections.
	mcpLines := 2 // blank + "MCP" header
	if len(s.MCPServers) == 0 {
		mcpLines++ // "  none"
	} else {
		mcpLines += len(s.MCPServers)
	}

	interactionLines := 2 // blank + "Interaction" header
	if s.QueueID != "" {
		interactionLines++
	}

	if len(s.SupportedHarnesses) > 0 {
		interactionLines++
	}

	if s.LastError != "" && s.Now.Sub(s.LastErrorTime) < 30*time.Second {
		interactionLines++
	}

	lists := []listInfo{
		{"Agents", agents},
		{"Skills", skills},
		{"Tools", tools},
	}

	// Calculate slots for each list.
	expanded := s.ExpandedSections
	slots := distributeListSlots(lists, expanded, len(lines), mcpLines+interactionLines, rows)

	for i, list := range lists {
		if len(list.items) == 0 {
			continue
		}

		isExpanded := expanded[list.title]

		maxItems := slots[i]
		if isExpanded {
			maxItems = len(list.items)
		}

		lines = append(lines, "", list.title)

		for j := 0; j < len(list.items) && j < maxItems; j++ {
			lines = append(lines, "  - "+list.items[j])
		}

		if isExpanded {
			targets = append(targets, SidebarClickTarget{Row: len(lines), Section: list.title})
			lines = append(lines, "  [collapse]")
		} else if len(list.items) > maxItems {
			targets = append(targets, SidebarClickTarget{Row: len(lines), Section: list.title})
			lines = append(lines, fmt.Sprintf("  +%d more (click)", len(list.items)-maxItems))
		}
	}

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

	lines = append(lines, "", "Interaction")

	if s.QueueID != "" {
		lines = append(lines, "  queue: "+s.QueueID)
	}

	if len(s.SupportedHarnesses) > 0 {
		lines = append(lines, "  harness: "+strings.Join(s.SupportedHarnesses, ", "))
	}

	if s.LastError != "" && s.Now.Sub(s.LastErrorTime) < 30*time.Second {
		msg := s.LastError
		if runewidth.StringWidth(msg) > 30 {
			msg = runewidth.Truncate(msg, 30, "...")
		}

		lines = append(lines, "  err: "+msg)
	}

	for len(lines) < rows {
		lines = append(lines, "")
	}

	if len(lines) > rows {
		lines = lines[:rows]
	}

	return lines, targets
}

type listInfo struct {
	title string
	items []string
}

// distributeListSlots allocates display slots to each non-empty list based on
// available terminal rows. It subtracts fixed overhead (bundle header lines,
// MCP/interaction lines, per-list headers) from the total rows and distributes
// remaining slots proportionally by list size.
func distributeListSlots(lists []listInfo, expanded map[string]bool, bundleLines, fixedBottomLines, rows int) []int {
	slots := make([]int, len(lists))

	// Count non-empty, non-expanded lists and total items needing slots.
	var nonEmptyCount int

	var totalItems int

	for i, list := range lists {
		if len(list.items) == 0 {
			continue
		}

		nonEmptyCount++

		if expanded[list.title] {
			// Expanded lists don't consume from the shared pool.
			slots[i] = len(list.items)
		} else {
			totalItems += len(list.items)
		}
	}

	if totalItems == 0 {
		return slots
	}

	// Each non-empty list takes 2 overhead lines: blank line + title.
	listOverhead := 2 * nonEmptyCount
	fixedLines := bundleLines + fixedBottomLines + listOverhead

	// Subtract space consumed by expanded lists (items + collapse line).
	for _, list := range lists {
		if len(list.items) > 0 && expanded[list.title] {
			fixedLines += len(list.items) + 1 // items + [collapse] line
		}
	}

	available := max(0, rows-fixedLines)

	// Proportional distribution among non-expanded lists.
	type candidate struct {
		index int
		count int
	}

	var candidates []candidate

	for i, list := range lists {
		if len(list.items) > 0 && !expanded[list.title] {
			candidates = append(candidates, candidate{i, len(list.items)})
		}
	}

	// First pass: floor allocation.
	distributed := 0

	for _, c := range candidates {
		share := available * c.count / totalItems
		// Each truncated list needs 1 slot for the "+N more" line.
		if share < c.count && share > 0 {
			share-- // reserve a slot for "+N more"
		}

		share = min(share, c.count)
		share = max(share, 0)
		slots[c.index] = share
		distributed += share
	}

	// Distribute remainder to the largest lists first.
	remainder := available - distributed
	if remainder > 0 {
		// Sort candidates by size descending for remainder distribution.
		sorted := make([]candidate, len(candidates))
		copy(sorted, candidates)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].count > sorted[j].count
		})

		for _, c := range sorted {
			if remainder <= 0 {
				break
			}

			current := slots[c.index]
			canAdd := c.count - current
			// If currently truncated with "+N more" line, adding 1 lets us
			// reclaim that line (no more "+N more" needed if we reach full).
			if current < c.count && canAdd > 0 {
				add := min(1, remainder)
				slots[c.index] += add
				remainder -= add
			}
		}
	}

	// Ensure minimum of 1 slot per non-expanded non-empty list when space allows.
	for _, c := range candidates {
		if slots[c.index] == 0 && available > 0 {
			slots[c.index] = 1
		}
	}

	return slots
}

func sidebarRow(content string, sidebarWidth int) string {
	maxContent := max(0, sidebarWidth-2)

	content = runewidth.Truncate(content, maxContent, "")

	body := " " + content
	body = render.PadRightVisible(body, sidebarWidth)

	return sidebarBG + sidebarFG + body + sidebarBorder + "│" + resetAll
}

func styleStatus(label string) string {
	switch label {
	case "Starting...":
		return yellow + bold + "Starting" + barReset
	case "Ready":
		return green + bold + "Ready" + barReset
	case "Connected":
		return green + bold + "Connected" + barReset
	case "Processing":
		return yellow + bold + "Processing" + barReset
	case "Error":
		return red + bold + "Error" + barReset
	default:
		return label
	}
}
