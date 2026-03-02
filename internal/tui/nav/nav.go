// Package nav implements the interactive TUI navigation for the root `mush` command.
package nav

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the interactive navigation TUI.
func Run(ctx context.Context) error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}

	return nil
}
