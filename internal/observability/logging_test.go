package observability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLogger_DefaultFileFallbackForInteractiveAuto(t *testing.T) {
	cfgRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)

	cfg := &Config{
		Level:          "info",
		Format:         "json",
		LogFile:        "",
		StderrMode:     "auto",
		InteractiveTTY: true,
		SessionID:      "session-test",
		CommandPath:    "mush bundle load",
		Version:        "test",
		Commit:         "abc123",
	}

	logger, cleanup, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	logger.Info("hello from test")

	if cleanup != nil {
		if closeErr := cleanup(); closeErr != nil {
			t.Fatalf("cleanup() error = %v", closeErr)
		}
	}

	logPath := filepath.Join(cfgRoot, "mush", "logs", "mush.log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}

	if len(data) == 0 {
		t.Fatalf("log file %q is empty", logPath)
	}
}

func TestRotateLogFile_RotatesAndKeepsBoundedBackups(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "mush.log")

	// Existing rotated files
	if err := os.WriteFile(logPath+".1", []byte("one"), 0o600); err != nil {
		t.Fatalf("write .1: %v", err)
	}

	if err := os.WriteFile(logPath+".2", []byte("two"), 0o600); err != nil {
		t.Fatalf("write .2: %v", err)
	}

	if err := os.WriteFile(logPath+".3", []byte("three"), 0o600); err != nil {
		t.Fatalf("write .3: %v", err)
	}

	// Current log above threshold
	if err := os.WriteFile(logPath, []byte("1234567890"), 0o600); err != nil {
		t.Fatalf("write current: %v", err)
	}

	if err := rotateLogFile(logPath, 5, 3); err != nil {
		t.Fatalf("rotateLogFile() error = %v", err)
	}

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected current log to be rotated away, stat err = %v", err)
	}

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected .1 to exist, stat err = %v", err)
	}

	if _, err := os.Stat(logPath + ".2"); err != nil {
		t.Fatalf("expected .2 to exist, stat err = %v", err)
	}

	if _, err := os.Stat(logPath + ".3"); err != nil {
		t.Fatalf("expected .3 to exist, stat err = %v", err)
	}

	data3, err := os.ReadFile(logPath + ".3")
	if err != nil {
		t.Fatalf("read .3: %v", err)
	}

	if string(data3) != "two" {
		t.Fatalf("backup retention ordering wrong: .3 = %q, want %q", string(data3), "two")
	}
}
