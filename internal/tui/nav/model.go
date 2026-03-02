package nav

import tea "github.com/charmbracelet/bubbletea"

type model struct{}

func newModel() model { return model{} }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	return "\n  Welcome to Mush Interactive\n\n  Press q to quit.\n\n"
}
