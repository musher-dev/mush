package nav

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/musher-dev/mush/internal/config"
)

// keyMap defines all key bindings for the TUI navigation.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Select   key.Binding
	Quit     key.Binding
	Back     key.Binding
	Tab      key.Binding
	Retry    key.Binding
	Help     key.Binding
	Search   key.Binding
	Install  key.Binding
	LoadMore key.Binding
	Status   key.Binding
}

// defaultKeyMap returns the resolved key bindings for the TUI.
func defaultKeyMap(cfg *config.Config) keyMap {
	resolved := config.DefaultKeybindings()
	if cfg != nil {
		resolved = cfg.Keybindings()
	}

	return keyMap{
		Up:       newBinding(resolved["up"], "up"),
		Down:     newBinding(resolved["down"], "down"),
		Left:     newBinding(resolved["left"], "left"),
		Right:    newBinding(resolved["right"], "right"),
		Select:   newBinding(resolved["select"], "select"),
		Quit:     newBinding(resolved["quit"], "quit"),
		Back:     newBinding(resolved["back"], "back"),
		Tab:      newBinding(resolved["tab"], "switch focus"),
		Retry:    newBinding(resolved["retry"], "retry"),
		Help:     newBinding(resolved["help"], "help"),
		Search:   newBinding(resolved["search"], "search"),
		Install:  newBinding(resolved["install"], "install"),
		LoadMore: newBinding(resolved["load_more"], "load more"),
		Status:   newBinding(resolved["status"], "status"),
	}
}

func newBinding(keys []string, desc string) key.Binding {
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(renderBindingKeys(keys), desc),
	)
}

func renderBindingKeys(keys []string) string {
	labels := make([]string, 0, len(keys))
	for _, k := range keys {
		labels = append(labels, displayKey(k))
	}

	return strings.Join(labels, "/")
}

func primaryHelpKey(binding key.Binding) string {
	keys := binding.Keys()
	if len(keys) == 0 {
		return ""
	}

	return displayKey(keys[0])
}

func displayKey(keyName string) string {
	switch strings.ToLower(strings.TrimSpace(keyName)) {
	case "up":
		return "\u2191"
	case "down":
		return "\u2193"
	case "left":
		return "\u2190"
	case "right":
		return "\u2192"
	case "ctrl+c":
		return "ctrl+c"
	default:
		return keyName
	}
}
