//go:build unix

package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallStopHook_MigratesLegacyAndRestores(t *testing.T) {
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

	prev, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("getwd failed: %v", getwdErr)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	chdirErr := os.Chdir(tmp)
	if chdirErr != nil {
		t.Fatalf("chdir failed: %v", chdirErr)
	}

	restore, err := installStopHook("/tmp/mush-test-signals")
	if err != nil {
		t.Fatalf("installStopHook failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings failed: %v", err)
	}

	var parsed map[string]interface{}
	unmarshalErr := json.Unmarshal(data, &parsed)
	if unmarshalErr != nil {
		t.Fatalf("parse settings failed: %v", unmarshalErr)
	}

	hooks := parsed["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	foundMushCommand := false
	for _, item := range stop {
		entry := item.(map[string]interface{})
		if _, ok := entry["hooks"].([]interface{}); !ok {
			t.Fatalf("expected hooks array entry, got: %#v", entry)
		}
		for _, rawHook := range entry["hooks"].([]interface{}) {
			hook := rawHook.(map[string]interface{})
			cmd, _ := hook["command"].(string)
			if strings.Contains(cmd, SignalFileName) {
				foundMushCommand = true
			}
		}
	}
	if !foundMushCommand {
		t.Fatalf("expected mush stop hook command to be present")
	}

	restoreErr := restore()
	if restoreErr != nil {
		t.Fatalf("restore failed: %v", restoreErr)
	}

	restored, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read restored settings failed: %v", err)
	}
	if !bytes.Equal(restored, original) {
		t.Fatalf("settings were not restored to original content")
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
					"matcher": map[string]interface{}{},
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
	writeErr := os.WriteFile(settingsPath, seedBytes, 0o600)
	if writeErr != nil {
		t.Fatalf("write failed: %v", writeErr)
	}

	prev, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("getwd failed: %v", getwdErr)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	chdirErr := os.Chdir(tmp)
	if chdirErr != nil {
		t.Fatalf("chdir failed: %v", chdirErr)
	}

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
		if matcher, ok := entry["matcher"]; ok {
			if _, isObject := matcher.(map[string]interface{}); isObject {
				t.Fatalf("expected stop matcher object to be removed, got %#v", matcher)
			}
		}
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
