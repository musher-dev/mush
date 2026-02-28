//go:build unix

package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallStopHook_RejectsLegacyEntryAndDoesNotMutate(t *testing.T) {
	tmp := t.TempDir()

	settingsPath := filepath.Join(tmp, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	original := []byte(`{
  "hooks": {
    "Stop": [
      {
        "matcher": "*",
        "command": "echo legacy"
      }
    ]
  }
}`)
	if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	t.Chdir(tmp)

	_, err := installStopHook("/tmp/mush-test-signals")
	if err == nil {
		t.Fatal("expected error for legacy Stop hook entry")
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings failed: %v", err)
	}

	if !bytes.Equal(after, original) {
		t.Fatalf("settings should remain unchanged on error")
	}
}

func TestInstallStopHook_RejectsInvalidMatcherTypeAndDoesNotMutate(t *testing.T) {
	tmp := t.TempDir()

	settingsPath := filepath.Join(tmp, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	original := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{
					"matcher": map[string]interface{}{},
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "echo ok"},
					},
				},
			},
		},
	}

	originalBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if writeErr := os.WriteFile(settingsPath, originalBytes, 0o600); writeErr != nil {
		t.Fatalf("write failed: %v", writeErr)
	}

	t.Chdir(tmp)

	_, err = installStopHook("/tmp/mush-test-signals")
	if err == nil {
		t.Fatal("expected error for invalid matcher object type")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings failed: %v", err)
	}

	if !bytes.Equal(data, originalBytes) {
		t.Fatalf("settings should remain unchanged on error")
	}
}

func TestInstallStopHook_DoesNotDuplicateExistingMushHook(t *testing.T) {
	tmp := t.TempDir()

	settingsPath := filepath.Join(tmp, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	mushCommand := `sh -c "if [ -n \"$MUSH_SIGNAL_DIR\" ]; then touch \"$MUSH_SIGNAL_DIR/` + SignalFileName + `\"; fi"`
	seed := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{
					"matcher": "*",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": mushCommand,
						},
					},
				},
			},
		},
	}

	seedBytes, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if writeErr := os.WriteFile(settingsPath, seedBytes, 0o600); writeErr != nil {
		t.Fatalf("write failed: %v", writeErr)
	}

	t.Chdir(tmp)

	restore, err := installStopHook("/tmp/mush-test-signals")
	if err != nil {
		t.Fatalf("installStopHook failed: %v", err)
	}

	defer func() { _ = restore() }()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse settings failed: %v", err)
	}

	hooks := parsed["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	count := 0

	for _, item := range stop {
		entry := item.(map[string]interface{})

		rawHooks, _ := entry["hooks"].([]interface{})
		for _, rawHook := range rawHooks {
			hook := rawHook.(map[string]interface{})
			if hook["command"] == mushCommand {
				count++
			}
		}
	}

	if count != 1 {
		t.Fatalf("expected exactly 1 mush hook command, got %d", count)
	}
}
