package config

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const keybindingsRoot = "keybindings"

var keybindingDefaults = map[string][]string{
	"up":        {"up", "k"},
	"down":      {"down", "j"},
	"left":      {"left", "h"},
	"right":     {"right", "l"},
	"select":    {"enter"},
	"quit":      {"q", "ctrl+c"},
	"back":      {"esc"},
	"tab":       {"tab"},
	"retry":     {"r"},
	"help":      {"?"},
	"search":    {"/"},
	"install":   {"i"},
	"load_more": {"l"},
	"status":    {","},
}

var keybindingActionOrder = []string{
	"up",
	"down",
	"left",
	"right",
	"select",
	"quit",
	"back",
	"tab",
	"retry",
	"help",
	"search",
	"install",
	"load_more",
	"status",
}

// KeybindingActions returns the supported keybinding action names.
func KeybindingActions() []string {
	return slices.Clone(keybindingActionOrder)
}

// DefaultKeybindings returns the default key list for each supported action.
func DefaultKeybindings() map[string][]string {
	out := make(map[string][]string, len(keybindingDefaults))
	for _, action := range keybindingActionOrder {
		out[action] = slices.Clone(keybindingDefaults[action])
	}

	return out
}

// IsKnownKeybindingAction reports whether action is supported.
func IsKnownKeybindingAction(action string) bool {
	_, ok := keybindingDefaults[action]

	return ok
}

// ParseKeybindingValue parses a config/CLI keybinding value into a validated string slice.
func ParseKeybindingValue(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("keybinding value cannot be empty")
	}

	if !strings.HasPrefix(raw, "[") {
		return ValidateKeybindingKeys([]string{raw})
	}

	var keys []string
	if err := yaml.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, fmt.Errorf("parse keybinding list: %w", err)
	}

	return ValidateKeybindingKeys(keys)
}

// ValidateKeybindingKeys normalizes and validates a set of keybinding tokens.
func ValidateKeybindingKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("keybinding list cannot be empty")
	}

	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))

	for _, key := range keys {
		token := normalizeKeybindingToken(key)
		if token == "" {
			return nil, fmt.Errorf("keybinding entries cannot be empty")
		}

		if _, exists := seen[token]; exists {
			return nil, fmt.Errorf("duplicate keybinding %q", token)
		}

		seen[token] = struct{}{}
		normalized = append(normalized, token)
	}

	return normalized, nil
}

// Keybindings returns the resolved keybinding map using defaults plus user overrides.
func (c *Config) Keybindings() map[string][]string {
	resolved := DefaultKeybindings()

	for _, action := range keybindingActionOrder {
		key := keybindingsRoot + "." + action
		if !c.v.IsSet(key) {
			continue
		}

		keys, err := coerceKeybindingKeys(c.v.Get(key))
		if err != nil {
			slog.Default().Warn(
				"invalid keybinding override",
				"component", "config",
				"event.type", "config.keybindings.warning",
				"action", action,
				"error", err.Error(),
			)

			continue
		}

		resolved[action] = keys
	}

	return resolved
}

func coerceKeybindingKeys(raw interface{}) ([]string, error) {
	switch value := raw.(type) {
	case []string:
		return ValidateKeybindingKeys(value)
	case []interface{}:
		keys := make([]string, 0, len(value))
		for _, item := range value {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("keybinding values must be strings")
			}

			keys = append(keys, str)
		}

		return ValidateKeybindingKeys(keys)
	case string:
		return ValidateKeybindingKeys([]string{value})
	default:
		return nil, fmt.Errorf("keybinding value must be a string or string list")
	}
}

func normalizeKeybindingToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
