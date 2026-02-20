package main

import (
	"bytes"
	"io"
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
