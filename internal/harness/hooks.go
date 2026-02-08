//go:build unix

package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

	if data, readErr := os.ReadFile(settingsPath); readErr == nil {
		original = data
		originalExists = true
	} else if !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("failed to read settings: %w", readErr)
	}

	settings := map[string]interface{}{}
	if originalExists && len(original) > 0 {
		if unmarshalErr := json.Unmarshal(original, &settings); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to parse settings: %w", unmarshalErr)
		}
	}

	hooks := map[string]interface{}{}
	if existingHooks, ok := settings["hooks"]; ok {
		typedHooks, ok := existingHooks.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("settings.hooks must be an object")
		}
		hooks = typedHooks
	}

	stopHooks := []interface{}{}
	if existingStop, ok := hooks["Stop"]; ok {
		typedStop, ok := existingStop.([]interface{})
		if !ok {
			return nil, fmt.Errorf("settings.hooks.Stop must be an array")
		}
		stopHooks = typedStop
	}

	command := fmt.Sprintf(
		"sh -c \"if [ -n \\\"$MUSH_SIGNAL_DIR\\\" ]; then touch \\\"$MUSH_SIGNAL_DIR/%s\\\"; fi\"",
		SignalFileName,
	)

	normalizedStopHooks := make([]interface{}, 0, len(stopHooks)+1)
	alreadyPresent := false
	for _, item := range stopHooks {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Normalize legacy hook entries:
		// {"matcher":"*","command":"..."} -> {"hooks":[{"type":"command","command":"..."}]}
		if cmd, ok := entry["command"].(string); ok {
			entry = map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": cmd,
					},
				},
			}
		} else {
			// Stop hooks in current Claude schema do not require matcher.
			// If matcher is malformed (e.g. object), drop it to avoid schema errors.
			if rawMatcher, ok := entry["matcher"]; ok {
				if _, isString := rawMatcher.(string); !isString {
					delete(entry, "matcher")
				}
			}
			if _, ok := entry["hooks"].([]interface{}); !ok {
				entry["hooks"] = []interface{}{}
			}
		}

		if rawHooks, ok := entry["hooks"].([]interface{}); ok {
			for _, rawHook := range rawHooks {
				hook, ok := rawHook.(map[string]interface{})
				if !ok {
					continue
				}
				if hookCmd, ok := hook["command"].(string); ok && hookCmd == command {
					alreadyPresent = true
				}
			}
		}

		normalizedStopHooks = append(normalizedStopHooks, entry)
	}

	if !alreadyPresent {
		normalizedStopHooks = append(normalizedStopHooks, map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": command,
				},
			},
		})
	}

	hooks["Stop"] = normalizedStopHooks
	settings["hooks"] = hooks

	if mkdirErr := os.MkdirAll(filepath.Dir(settingsPath), 0o755); mkdirErr != nil {
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
			return os.WriteFile(settingsPath, original, 0o600)
		}
		if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return restore, nil
}
