//go:build unix

package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type settingsFile struct {
	Hooks map[string][]hookEntry `json:"hooks,omitempty"`
	extra map[string]json.RawMessage
}

func (s *settingsFile) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal settings json: %w", err)
	}

	s.extra = raw
	if rawHooks, ok := raw["hooks"]; ok {
		var hooks map[string][]hookEntry
		if err := json.Unmarshal(rawHooks, &hooks); err != nil {
			return fmt.Errorf("settings.hooks must be an object")
		}

		s.Hooks = hooks
		delete(s.extra, "hooks")
	}

	return nil
}

func (s settingsFile) MarshalJSON() ([]byte, error) {
	raw := make(map[string]json.RawMessage, len(s.extra)+1)
	for key, value := range s.extra {
		raw[key] = value
	}

	if s.Hooks != nil {
		encoded, err := json.Marshal(s.Hooks)
		if err != nil {
			return nil, fmt.Errorf("marshal stop hooks: %w", err)
		}

		raw["hooks"] = encoded
	}

	encodedRaw, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal settings json: %w", err)
	}

	return encodedRaw, nil
}

type hookEntry struct {
	Matcher interface{}   `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks,omitempty"`
	Command string        `json:"command,omitempty"`
}

type hookCommand struct {
	Type    string `json:"type,omitempty"`
	Command string `json:"command,omitempty"`
}

// installStopHook ensures a Stop hook is installed for completion signaling.
// It returns a restore function to revert any changes on exit.
func installStopHook(signalDir string) (func() error, error) {
	if signalDir == "" {
		return nil, fmt.Errorf("signal directory is required")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	settingsPath := filepath.Join(cwd, ".claude", "settings.local.json")

	var original []byte

	originalExists := false

	settings := settingsFile{}

	if data, readErr := os.ReadFile(settingsPath); readErr == nil { //nolint:gosec // G304: path from known .claude directory
		original = data
		originalExists = true

		if len(original) > 0 {
			if unmarshalErr := json.Unmarshal(original, &settings); unmarshalErr != nil {
				return nil, fmt.Errorf("failed to parse settings: %w", unmarshalErr)
			}
		}
	} else if !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("failed to read settings: %w", readErr)
	}

	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]hookEntry)
	}

	stopHooks := settings.Hooks["Stop"]

	command := fmt.Sprintf(
		"sh -c \"if [ -n \\\"$MUSH_SIGNAL_DIR\\\" ]; then touch \\\"$MUSH_SIGNAL_DIR/%s\\\"; fi\"",
		SignalFileName,
	)

	normalizedStopHooks := make([]hookEntry, 0, len(stopHooks)+1)
	alreadyPresent := false

	for _, item := range stopHooks {
		// Normalize legacy hook entries:
		// {"matcher":"*","command":"..."} -> {"hooks":[{"type":"command","command":"..."}]}
		if item.Command != "" {
			item = hookEntry{
				Hooks: []hookCommand{
					{
						Type:    "command",
						Command: item.Command,
					},
				},
			}
		} else {
			// Stop hooks in current Claude schema do not require matcher.
			// If matcher is malformed (e.g. object), drop it to avoid schema errors.
			if item.Matcher != nil {
				if _, isString := item.Matcher.(string); !isString {
					item.Matcher = nil
				}
			}

			if item.Hooks == nil {
				item.Hooks = []hookCommand{}
			}
		}

		for _, hook := range item.Hooks {
			if hook.Command == command {
				alreadyPresent = true
			}
		}

		normalizedStopHooks = append(normalizedStopHooks, item)
	}

	if !alreadyPresent {
		normalizedStopHooks = append(normalizedStopHooks, hookEntry{
			Hooks: []hookCommand{
				{
					Type:    "command",
					Command: command,
				},
			},
		})
	}

	settings.Hooks["Stop"] = normalizedStopHooks

	if mkdirErr := os.MkdirAll(filepath.Dir(settingsPath), 0o755); mkdirErr != nil { //nolint:gosec // G301: .claude dir needs 0o755 for compatibility
		return nil, fmt.Errorf("failed to create .claude directory: %w", mkdirErr)
	}

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, updated, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write settings: %w", err)
	}

	restore := func() error {
		if originalExists {
			if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
				return fmt.Errorf("restore original settings file: %w", err)
			}

			return nil
		}

		if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove temporary settings file: %w", err)
		}

		return nil
	}

	return restore, nil
}
