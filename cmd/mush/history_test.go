package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
	"github.com/musher-dev/mush/internal/transcript"
)

func TestRenderTranscriptEventsFiltersAndAdvancesWatermark(t *testing.T) {
	var stdout bytes.Buffer

	out := output.NewWriter(&stdout, &stdout, &terminal.Info{IsTTY: false})
	events := []transcript.Event{
		{Seq: 1, Text: "\x1b[31mhello\x1b[0m\n"},
		{Seq: 2, Text: "skip me\n"},
		{Seq: 3, Text: "hello again\n"},
	}

	lastSeq := renderTranscriptEvents(out, events, 0, "hello", false)
	if lastSeq != 3 {
		t.Fatalf("lastSeq = %d, want 3", lastSeq)
	}

	got := stdout.String()
	if strings.Contains(got, "\x1b[31m") {
		t.Fatalf("expected ANSI to be stripped, got %q", got)
	}

	if strings.Contains(got, "skip me") {
		t.Fatalf("expected search filtering to suppress unmatched lines, got %q", got)
	}

	if !strings.Contains(got, "hello\n") || !strings.Contains(got, "hello again\n") {
		t.Fatalf("expected matching lines to be printed, got %q", got)
	}
}
