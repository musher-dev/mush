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
	// Check if stdout is a terminal
	return term.IsTerminal(int(os.Stdout.Fd())) && !p.out.NoInput
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

// SelectHabitat prompts the user to select a habitat from a list.
func SelectHabitat(habitats []client.HabitatSummary, out *output.Writer) (*client.HabitatSummary, error) {
	out.Println()
	out.Print("Available habitats:\n\n")

	for i, h := range habitats {
		// Format: [1] local-dev    (Local development)
		status := ""
		switch h.Status {
		case "online":
			status = "[online]"
		case "offline":
			status = "[offline]"
		case "degraded":
			status = "[degraded]"
		}
		out.Print("  [%d] %-20s %s %s\n", i+1, h.Slug, h.Name, status)
	}

	out.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		if len(habitats) == 1 {
			out.Print("Select habitat [1]: ")
		} else {
			out.Print("Select habitat [1-%d]: ", len(habitats))
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(habitats) {
			out.Warning("Invalid selection. Please enter a number between 1 and %d", len(habitats))
			continue
		}

		return &habitats[num-1], nil
	}
}

// SelectQueue prompts the user to select a queue from a list.
func SelectQueue(queues []client.QueueSummary, out *output.Writer) (*client.QueueSummary, error) {
	out.Println()
	out.Print("Available queues:\n\n")

	for i, q := range queues {
		status := ""
		switch q.Status {
		case "active":
			status = "[active]"
		case "paused":
			status = "[paused]"
		case "draining":
			status = "[draining]"
		}
		out.Print("  [%d] %-20s %s %s\n", i+1, q.Slug, q.Name, status)
	}

	out.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		if len(queues) == 1 {
			out.Print("Select queue [1]: ")
		} else {
			out.Print("Select queue [1-%d]: ", len(queues))
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(queues) {
			out.Warning("Invalid selection. Please enter a number between 1 and %d", len(queues))
			continue
		}

		return &queues[num-1], nil
	}
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
