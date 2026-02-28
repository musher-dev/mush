//go:build unix

package harness

import (
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/client"
)

func TestGetPromptFromJob_StrictExecutionContract(t *testing.T) {
	t.Run("requires execution config", func(t *testing.T) {
		_, err := getPromptFromJob(&client.Job{})
		if err == nil || !strings.Contains(err.Error(), "missing execution config") {
			t.Fatalf("err = %v, want missing execution config", err)
		}
	})

	t.Run("requires rendered instruction", func(t *testing.T) {
		_, err := getPromptFromJob(&client.Job{
			Execution: &client.ExecutionConfig{},
		})
		if err == nil || !strings.Contains(err.Error(), "missing execution.renderedInstruction") {
			t.Fatalf("err = %v, want missing execution.renderedInstruction", err)
		}
	})

	t.Run("uses rendered instruction", func(t *testing.T) {
		got, err := getPromptFromJob(&client.Job{
			Execution: &client.ExecutionConfig{RenderedInstruction: "do work"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "do work" {
			t.Fatalf("prompt = %q, want %q", got, "do work")
		}
	})
}

func TestGetBashCommandFromJob_StrictExecutionContract(t *testing.T) {
	t.Run("requires execution config", func(t *testing.T) {
		_, err := getBashCommandFromJob(&client.Job{})
		if err == nil || !strings.Contains(err.Error(), "missing execution config") {
			t.Fatalf("err = %v, want missing execution config", err)
		}
	})

	t.Run("requires rendered instruction", func(t *testing.T) {
		_, err := getBashCommandFromJob(&client.Job{
			Execution: &client.ExecutionConfig{},
		})
		if err == nil || !strings.Contains(err.Error(), "missing execution.renderedInstruction") {
			t.Fatalf("err = %v, want missing execution.renderedInstruction", err)
		}
	})

	t.Run("uses rendered instruction", func(t *testing.T) {
		got, err := getBashCommandFromJob(&client.Job{
			Execution: &client.ExecutionConfig{RenderedInstruction: "echo hi"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "echo hi" {
			t.Fatalf("command = %q, want %q", got, "echo hi")
		}
	})
}
