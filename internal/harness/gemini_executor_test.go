//go:build unix

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func TestGeminiSetup_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	exec := &GeminiExecutor{}

	err := exec.Setup(t.Context(), &SetupOptions{})
	if err == nil || !strings.Contains(err.Error(), "gemini CLI not found") {
		t.Fatalf("Setup() err = %v, want binary not found", err)
	}
}

func TestGeminiExecute_SuccessWithMCPConfig(t *testing.T) {
	installFakeGemini(t, `#!/bin/sh
mode="interactive"
for arg in "$@"; do
  if [ "$arg" = "-p" ]; then
    mode="prompt"
  fi
done

if [ -n "$MUSH_GEMINI_TEST_FILE" ]; then
  echo "PWD=$PWD" > "$MUSH_GEMINI_TEST_FILE"
  echo "ARGS=$*" >> "$MUSH_GEMINI_TEST_FILE"
  echo "CFG=$GEMINI_CLI_CONFIG_DIR" >> "$MUSH_GEMINI_TEST_FILE"
  if [ -n "$GEMINI_CLI_CONFIG_DIR" ] && [ -f "$GEMINI_CLI_CONFIG_DIR/settings.json" ]; then
    cat "$GEMINI_CLI_CONFIG_DIR/settings.json" >> "$MUSH_GEMINI_TEST_FILE"
  fi
fi

if [ "$mode" = "prompt" ]; then
  echo "gemini ok"
  exit 0
fi

sleep 30
`)

	exp := time.Now().Add(10 * time.Minute)
	cfg := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok",
					TokenType:   "bearer",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	exec := &GeminiExecutor{}
	if err := exec.Setup(t.Context(), &SetupOptions{RunnerConfig: cfg}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	workDir := t.TempDir()
	tracePath := filepath.Join(t.TempDir(), "gemini-trace.txt")

	job := &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test gemini"},
		Execution: &client.ExecutionConfig{
			RenderedInstruction: "prompt for gemini",
			WorkingDirectory:    workDir,
			Environment: map[string]string{
				"MUSH_GEMINI_TEST_FILE": tracePath,
			},
		},
	}

	result, err := exec.Execute(t.Context(), job)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "gemini ok" {
		t.Fatalf("output = %q, want gemini ok", output)
	}

	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}

	trace := string(traceData)
	if !strings.Contains(trace, "PWD="+workDir) {
		t.Fatalf("trace missing working directory, trace=%q", trace)
	}

	if !strings.Contains(trace, "--approval-mode yolo") {
		t.Fatalf("trace missing approval-mode args, trace=%q", trace)
	}

	if !strings.Contains(trace, "--sandbox workspace-write") {
		t.Fatalf("trace missing sandbox args, trace=%q", trace)
	}

	if !strings.Contains(trace, "\"mcpServers\"") {
		t.Fatalf("trace missing mcpServers config, trace=%q", trace)
	}
}

func TestGeminiExecute_Timeout(t *testing.T) {
	installFakeGemini(t, `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "-p" ]; then
    sleep 2
    exit 0
  fi
done
sleep 30
`)

	exec := &GeminiExecutor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, geminiTestJob("prompt for gemini"))
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout")
	}

	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("err type = %T, want *ExecError", err)
	}

	if execErr.Reason != "timeout" {
		t.Fatalf("Reason = %q, want timeout", execErr.Reason)
	}
}

func TestGeminiExecute_BundleModeQueueRejected(t *testing.T) {
	exec := &GeminiExecutor{
		opts: SetupOptions{
			BundleDir: "/tmp/some-bundle",
		},
	}

	_, err := exec.Execute(t.Context(), geminiTestJob("prompt"))
	if err == nil {
		t.Fatal("Execute() error = nil, want bundle rejection")
	}

	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("err type = %T, want *ExecError", err)
	}

	if execErr.Reason != "execution_error" {
		t.Fatalf("Reason = %q, want execution_error", execErr.Reason)
	}
}

func TestGeminiSetup_BundleInteractiveLifecycle(t *testing.T) {
	installFakeGemini(t, `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "-p" ]; then
    echo "prompt mode"
    exit 0
  fi
done

echo "interactive session"
sleep 30
`)

	exec := &GeminiExecutor{}

	err := exec.Setup(t.Context(), &SetupOptions{
		BundleDir:  t.TempDir(),
		TermWidth:  120,
		TermHeight: 30,
	})
	if err != nil {
		if strings.Contains(err.Error(), "open /dev/ptmx: permission denied") {
			t.Skip("PTY unavailable in sandbox")
		}

		t.Fatalf("Setup() error = %v", err)
	}

	if _, writeErr := exec.WriteInput([]byte("help\n")); writeErr != nil {
		t.Fatalf("WriteInput() error = %v", writeErr)
	}

	if err := exec.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}

	exec.Resize(40, 100)
	exec.Teardown()
}

func installFakeGemini(t *testing.T, script string) {
	t.Helper()

	binDir := t.TempDir()

	path := filepath.Join(binDir, "gemini")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gemini: %v", err)
	}

	sep := string(os.PathListSeparator)
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%s%s", binDir, sep, currentPath))
}

func geminiTestJob(prompt string) *client.Job {
	return &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test job"},
		Execution: &client.ExecutionConfig{
			RenderedInstruction: prompt,
		},
	}
}
