package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
	"github.com/musher-dev/mush/internal/testutil"
)

func testWriter() (*output.Writer, *bytes.Buffer) {
	var buf bytes.Buffer

	term := &terminal.Info{IsTTY: false, NoColor: true, Width: 80, Height: 24}

	return output.NewWriter(&buf, &buf, term), &buf
}

func TestConfigList_Empty_Golden(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MUSH_HISTORY_DIR", "/tmp/mush-history")

	out, buf := testWriter()
	cmd := newConfigListCmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config list should succeed: %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "config_list_empty.golden")
}

func TestConfigGet_Set_Golden(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MUSH_API_URL", "https://custom.api.dev")

	out, buf := testWriter()
	cmd := newConfigGetCmd()
	cmd.SetArgs([]string{"api.url"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get should succeed: %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "config_get_set.golden")
}

func TestConfigGet_Unset_Golden(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out, buf := testWriter()
	cmd := newConfigGetCmd()
	cmd.SetArgs([]string{"custom.key"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get should succeed for unset key: %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "config_get_unset.golden")
}

func TestConfigSet_KeybindingsListValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))

	out, _ := testWriter()
	cmd := newConfigSetCmd()
	cmd.SetArgs([]string{"keybindings.up", `["up","w"]`})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set should succeed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mush", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "keybindings:") || !strings.Contains(content, "- up") || !strings.Contains(content, "- w") {
		t.Fatalf("config.yaml missing keybinding list, got:\n%s", content)
	}
}

func TestConfigSet_RejectsUnknownKeybindingAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	out, _ := testWriter()
	cmd := newConfigSetCmd()
	cmd.SetArgs([]string{"keybindings.unknown", "x"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err == nil {
		t.Fatal("config set should fail for unknown keybinding action")
	}
}

func TestConfigGet_KeybindingsValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))

	out, _ := testWriter()
	setCmd := newConfigSetCmd()
	setCmd.SetArgs([]string{"keybindings.status", `["g"]`})
	setCmd.SetOut(io.Discard)
	setCmd.SetErr(io.Discard)
	setCmd.SetContext(out.WithContext(t.Context()))

	if err := setCmd.Execute(); err != nil {
		t.Fatalf("config set should succeed: %v", err)
	}

	out, buf := testWriter()
	getCmd := newConfigGetCmd()
	getCmd.SetArgs([]string{"keybindings.status"})
	getCmd.SetOut(io.Discard)
	getCmd.SetErr(io.Discard)
	getCmd.SetContext(out.WithContext(t.Context()))

	if err := getCmd.Execute(); err != nil {
		t.Fatalf("config get should succeed: %v", err)
	}

	if !strings.Contains(buf.String(), "keybindings.status = [g]") {
		t.Fatalf("config get output = %q, want keybindings.status list", buf.String())
	}
}
