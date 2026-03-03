// Package nav implements the interactive TUI navigation for the root `mush` command.
package nav

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run launches the interactive navigation TUI.
// It accepts dependencies for context-aware rendering and returns a result
// indicating any action the user selected (e.g. bundle load).
func Run(ctx context.Context, deps *Dependencies) (*Result, error) {
	mdl := newModel(ctx, deps)

	p := tea.NewProgram(mdl, tea.WithAltScreen(), tea.WithContext(ctx))

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run TUI: %w", err)
	}

	if m, ok := finalModel.(*model); ok && m.result != nil {
		return m.result, nil
	}

	return &Result{Action: ActionNone}, nil
}
