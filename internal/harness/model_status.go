//go:build unix

package harness

import (
	"fmt"
	"strings"
	"time"
)

// drawStatusBar renders the status bar at the top of the screen.
func (m *RootModel) drawStatusBar() {
	m.statusMu.Lock()
	habitatID := m.habitatID
	queueID := m.queueID
	status := m.status
	completed := m.completed
	failed := m.failed
	lastHeartbeat := m.lastHeartbeat
	lastError := m.lastError
	lastErrorTime := m.lastErrorTime
	m.statusMu.Unlock()

	m.jobMu.Lock()

	jobID := ""
	if m.currentJob != nil {
		jobID = m.currentJob.ID
	}
	m.jobMu.Unlock()

	hbAge := formatHeartbeatAge(lastHeartbeat)
	renderedStatus := renderStatus(status)

	// Save cursor and move to top.
	var builder strings.Builder
	builder.WriteString(escSaveCursor)
	fmt.Fprintf(&builder, escMoveTo, 1, 1)

	// Line 1: MUSH HARNESS | Habitat | Status | Job.
	line1Parts := []string{
		"\x1b[1mMUSH HARNESS\x1b[0m",
		fmt.Sprintf("Habitat: \x1b[1m%s\x1b[0m", habitatID),
		fmt.Sprintf("Status: %s", renderedStatus),
	}
	if m.isCopyMode() {
		line1Parts = append(line1Parts, "Mode: \x1b[33mCOPY\x1b[0m")
	} else {
		line1Parts = append(line1Parts, "Mode: \x1b[32mLIVE\x1b[0m")
	}

	if jobID != "" {
		line1Parts = append(line1Parts, fmt.Sprintf("Job: \x1b[1m%s\x1b[0m", jobID))
	} else {
		line1Parts = append(line1Parts, "\x1b[90mJob: (waiting...)\x1b[0m")
	}

	line1 := strings.Join(line1Parts, " \x1b[90m|\x1b[0m ")

	// Render line 1.
	builder.WriteString(escClearLine)
	builder.WriteString("\x1b[48;5;236m\x1b[38;5;252m ")
	builder.WriteString(line1)

	padding := m.width - m.visibleLength(line1) - 2
	if padding > 0 {
		builder.WriteString(strings.Repeat(" ", padding))
	}

	builder.WriteString(" \x1b[0m")

	// Move to line 2.
	fmt.Fprintf(&builder, escMoveTo, 2, 1)

	// Line 2: Heartbeat | Queue | Completed | Failed | Last Error.
	line2Parts := []string{
		fmt.Sprintf("HB: \x1b[1m%s\x1b[0m", hbAge),
		fmt.Sprintf("Queue ID: \x1b[1m%s\x1b[0m", queueID),
		fmt.Sprintf("Done: \x1b[1m%d\x1b[0m", completed),
		fmt.Sprintf("Failed: \x1b[1m%d\x1b[0m", failed),
	}

	if lastError != "" && time.Since(lastErrorTime) < 30*time.Second {
		errorStr := lastError
		if len(errorStr) > 40 {
			errorStr = errorStr[:37] + "..."
		}

		line2Parts = append(line2Parts, fmt.Sprintf("Error: \x1b[31m%s\x1b[0m", errorStr))
	}

	line2 := strings.Join(line2Parts, " \x1b[90m|\x1b[0m ")

	// Render line 2.
	builder.WriteString(escClearLine)
	builder.WriteString("\x1b[48;5;236m\x1b[38;5;252m ")
	builder.WriteString(line2)

	padding = m.width - m.visibleLength(line2) - 2
	if padding > 0 {
		builder.WriteString(strings.Repeat(" ", padding))
	}

	builder.WriteString(" \x1b[0m")

	// Restore cursor.
	builder.WriteString(escRestoreCursor)

	m.termWriteString(builder.String())
}

func renderStatus(status ConnectionStatus) string {
	switch status {
	case StatusConnected:
		return "\x1b[32m\x1b[1mConnected\x1b[0m"
	case StatusProcessing:
		return "\x1b[33m\x1b[1mProcessing\x1b[0m"
	case StatusError:
		return "\x1b[31m\x1b[1mError\x1b[0m"
	default:
		return status.String()
	}
}

func formatHeartbeatAge(lastHeartbeat time.Time) string {
	age := time.Since(lastHeartbeat)
	if age < time.Second {
		return "<1s ago"
	}

	if age < time.Minute {
		return fmt.Sprintf("%ds ago", int(age.Seconds()))
	}

	return fmt.Sprintf("%dm ago", int(age.Minutes()))
}

// visibleLength returns the visible length of a string, excluding ANSI codes.
func (m *RootModel) visibleLength(value string) int {
	length := 0
	inEscape := false

	for _, r := range value {
		if r == '\x1b' {
			inEscape = true
			continue
		}

		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}

			continue
		}

		length++
	}

	return length
}
