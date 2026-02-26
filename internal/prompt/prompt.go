// Package prompt provides interactive prompts for the Mush CLI.
package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/output"
	"golang.org/x/term"
)

// Prompter handles interactive prompts.
type Prompter struct {
	out    *output.Writer
	reader *bufio.Reader
}

// New creates a new Prompter.
func New(out *output.Writer) *Prompter {
	return &Prompter{
		out:    out,
		reader: bufio.NewReader(os.Stdin),
	}
}

// CanPrompt returns true if interactive prompts are available.
func (p *Prompter) CanPrompt() bool {
	// Check if stdin is a terminal (stdin is where we read interactive input)
	return term.IsTerminal(int(os.Stdin.Fd())) && !p.out.NoInput
}

// Confirm prompts for a yes/no confirmation.
func (p *Prompter) Confirm(message string, defaultValue bool) (bool, error) {
	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}

	p.out.Print("%s [%s]: ", message, defaultStr)

	input, err := p.reader.ReadString('\n')
	if err != nil {
		return defaultValue, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultValue, nil
	}

	return input == "y" || input == "yes", nil
}

// Password prompts for a password (hidden input).
func (p *Prompter) Password(prompt string) (string, error) {
	p.out.Print("%s: ", prompt)

	// Read password without echo
	password, err := term.ReadPassword(int(os.Stdin.Fd()))

	p.out.Println() // Print newline after password input

	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	return string(password), nil
}

// Select prompts the user to select from a list of options.
func (p *Prompter) Select(message string, options []string) (int, error) {
	p.out.Println(message)

	for i, opt := range options {
		p.out.Print("  [%d] %s\n", i+1, opt)
	}

	p.out.Println()

	for {
		p.out.Print("Select [1-%d]: ", len(options))

		input, err := p.reader.ReadString('\n')
		if err != nil {
			return -1, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(options) {
			p.out.Warning("Invalid selection. Please enter a number between 1 and %d", len(options))
			continue
		}

		return num - 1, nil
	}
}

type selectableSummary interface {
	GetSlug() string
	GetName() string
	GetStatus() string
}

type habitatOption struct {
	client.HabitatSummary
}

func (o *habitatOption) GetSlug() string {
	return o.Slug
}

func (o *habitatOption) GetName() string {
	return o.Name
}

func (o *habitatOption) GetStatus() string {
	switch o.Status {
	case "online":
		return "[online]"
	case "offline":
		return "[offline]"
	case "degraded":
		return "[degraded]"
	default:
		return ""
	}
}

type queueOption struct {
	client.QueueSummary
}

func (o *queueOption) GetSlug() string {
	return o.Slug
}

func (o *queueOption) GetName() string {
	return o.Name
}

func (o *queueOption) GetStatus() string {
	switch o.Status {
	case "active":
		return "[active]"
	case "paused":
		return "[paused]"
	case "draining":
		return "[draining]"
	default:
		return ""
	}
}

func selectSummary[T selectableSummary](title, itemLabel string, entries []T, out *output.Writer) (int, error) {
	// Try arrow-key selection when stdin is a terminal.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		idx, err := selectArrowKey(title, entries)
		if err == nil {
			return idx, nil
		}
		// Fall through to numbered input on any error (e.g. raw mode failure).
	}

	return selectNumbered(title, itemLabel, entries, out)
}

// selectNumbered is the classic numbered-input selection mode.
func selectNumbered[T selectableSummary](title, itemLabel string, entries []T, out *output.Writer) (int, error) {
	out.Println()
	out.Print("Available %s:\n\n", title)

	for index, entry := range entries {
		out.Print("  [%d] %-20s %s %s\n", index+1, entry.GetSlug(), entry.GetName(), entry.GetStatus())
	}

	out.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		if len(entries) == 1 {
			out.Print("Select %s [1]: ", itemLabel)
		} else {
			out.Print("Select %s [1-%d]: ", itemLabel, len(entries))
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return -1, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		selectedNumber, err := strconv.Atoi(input)
		if err != nil || selectedNumber < 1 || selectedNumber > len(entries) {
			out.Warning("Invalid selection. Please enter a number between 1 and %d", len(entries))
			continue
		}

		return selectedNumber - 1, nil
	}
}

// errCanceled is returned when the user cancels arrow-key selection.
var errCanceled = fmt.Errorf("selection canceled")

// selectArrowKey provides arrow-key navigation for TTY selection.
// Up/Down moves the cursor, Enter confirms, Esc/Ctrl+C cancels.
func selectArrowKey[T selectableSummary](title string, entries []T) (int, error) {
	stdinFd := int(os.Stdin.Fd())

	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return -1, fmt.Errorf("raw mode: %w", err)
	}

	defer func() { _ = term.Restore(stdinFd, oldState) }()

	selected := 0
	totalItems := len(entries)

	// Build display lines for each entry.
	lines := make([]string, totalItems)
	for i, entry := range entries {
		lines[i] = fmt.Sprintf("%-20s %s %s", entry.GetSlug(), entry.GetName(), entry.GetStatus())
	}

	// Write initial header + list.
	writeStr := func(s string) { _, _ = os.Stdout.WriteString(s) }

	writeStr(fmt.Sprintf("\r\nAvailable %s:\r\n\r\n", title))

	drawList := func() {
		for i := range totalItems {
			if i == selected {
				writeStr(fmt.Sprintf("  \x1b[1m> %s\x1b[0m\r\n", lines[i]))
			} else {
				writeStr(fmt.Sprintf("    %s\r\n", lines[i]))
			}
		}

		writeStr("\r\n  Use \x1b[1m↑/↓\x1b[0m to navigate, \x1b[1mEnter\x1b[0m to confirm, \x1b[1mEsc\x1b[0m to cancel")
	}

	drawList()

	// Move cursor up to redraw position (totalItems lines + 1 hint line).
	moveUp := func() {
		writeStr(fmt.Sprintf("\x1b[%dA\r", totalItems+1))
	}

	buf := make([]byte, 3) //nolint:mnd // ANSI escape sequences are 3 bytes

	for {
		n, readErr := os.Stdin.Read(buf)
		if readErr != nil {
			return -1, fmt.Errorf("read input: %w", readErr)
		}

		switch {
		case n == 1 && buf[0] == '\r': // Enter
			// Move past the list to avoid overwriting.
			writeStr("\r\n\r\n")

			return selected, nil

		case n == 1 && buf[0] == 0x1b: // Esc
			writeStr("\r\n\r\n")

			return -1, errCanceled

		case n == 1 && buf[0] == 0x03: // Ctrl+C
			writeStr("\r\n\r\n")

			return -1, errCanceled

		case n == 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'A': // Up
			if selected > 0 {
				selected--

				moveUp()
				drawList()
			}

		case n == 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'B': // Down
			if selected < totalItems-1 {
				selected++

				moveUp()
				drawList()
			}
		}
	}
}

// SelectHabitat prompts the user to select a habitat from a list.
func SelectHabitat(habitats []client.HabitatSummary, out *output.Writer) (*client.HabitatSummary, error) {
	options := make([]*habitatOption, 0, len(habitats))
	for _, habitat := range habitats {
		options = append(options, &habitatOption{HabitatSummary: habitat})
	}

	selectedIndex, err := selectSummary("habitats", "habitat", options, out)
	if err != nil {
		return nil, err
	}

	return &habitats[selectedIndex], nil
}

// SelectQueue prompts the user to select a queue from a list.
func SelectQueue(queues []client.QueueSummary, out *output.Writer) (*client.QueueSummary, error) {
	options := make([]*queueOption, 0, len(queues))
	for _, queue := range queues {
		options = append(options, &queueOption{QueueSummary: queue})
	}

	selectedIndex, err := selectSummary("queues", "queue", options, out)
	if err != nil {
		return nil, err
	}

	return &queues[selectedIndex], nil
}

// APIKey prompts the user for an API key.
func APIKey(out *output.Writer) (string, error) {
	out.Print("Enter your API key: ")

	reader := bufio.NewReader(os.Stdin)

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	return strings.TrimSpace(input), nil
}
