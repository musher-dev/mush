//go:build unix

package copilot

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

func TestCopilotSetup_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	exec := &Executor{}

	err := exec.Setup(t.Context(), &SetupOptions{})
	if err == nil || !strings.Contains(err.Error(), "copilot CLI not found") {
		t.Fatalf("Setup() err = %v, want binary not found", err)
	}
}

func TestCopilotExecute_SuccessWithMCPConfig(t *testing.T) {
	installFakeCopilot(t, `#!/bin/sh
mode="interactive"
prev=""
for arg in "$@"; do
  if [ "$prev" = "-p" ]; then
    mode="prompt"
  fi
  prev="$arg"
done

if [ -n "$MUSH_COPILOT_TEST_FILE" ]; then
  echo "PWD=$PWD" > "$MUSH_COPILOT_TEST_FILE"
  echo "ARGS=$*" >> "$MUSH_COPILOT_TEST_FILE"
fi

if [ "$mode" = "prompt" ]; then
  echo '{"type":"assistant","message":{"content":"first line"}}'
  echo '{"content":[{"type":"text","text":"second line"}]}'
  exit 0
fi

echo "interactive"
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

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{RunnerConfig: cfg}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer exec.Teardown()

	workDir := t.TempDir()
	tracePath := filepath.Join(t.TempDir(), "copilot-trace.txt")

	job := &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test copilot"},
		Execution: &client.ExecutionConfig{
			RenderedInstruction: "prompt for copilot",
			WorkingDirectory:    workDir,
			Environment: map[string]string{
				"MUSH_COPILOT_TEST_FILE": tracePath,
			},
		},
	}

	result, err := exec.Execute(t.Context(), job)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "first line\nsecond line" {
		t.Fatalf("output = %q, want parsed text", output)
	}

	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}

	trace := string(traceData)
	if !strings.Contains(trace, "PWD="+workDir) {
		t.Fatalf("trace missing working directory, trace=%q", trace)
	}

	if !strings.Contains(trace, "--allow-all-tools") {
		t.Fatalf("trace missing allow-all-tools args, trace=%q", trace)
	}

	if !strings.Contains(trace, "--additional-mcp-config @") {
		t.Fatalf("trace missing @mcp-config arg, trace=%q", trace)
	}
}

func TestCopilotExecute_FallbackToRawOutput(t *testing.T) {
	installFakeCopilot(t, `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "-p" ]; then
    echo "plain output from copilot"
    exit 0
  fi
done

sleep 30
`)

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer exec.Teardown()

	result, err := exec.Execute(t.Context(), copilotTestJob("prompt for copilot"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "plain output from copilot" {
		t.Fatalf("output = %q, want raw fallback output", output)
	}
}

func TestCopilotExecute_Timeout(t *testing.T) {
	installFakeCopilot(t, `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "-p" ]; then
    sleep 2
    exit 0
  fi
done

sleep 30
`)

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer exec.Teardown()

	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, copilotTestJob("prompt for copilot"))
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

func TestCopilotExecute_BundleModeQueueRejected(t *testing.T) {
	exec := &Executor{
		opts: SetupOptions{
			BundleDir: "/tmp/some-bundle",
		},
	}

	_, err := exec.Execute(t.Context(), copilotTestJob("prompt"))
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

func TestCopilotSetup_BundleInteractiveLifecycle(t *testing.T) {
	installFakeCopilot(t, `#!/bin/sh
prev=""
for arg in "$@"; do
  if [ "$prev" = "-p" ]; then
    echo "prompt mode"
    exit 0
  fi
  prev="$arg"
done

echo "interactive session"
sleep 30
`)

	exec := &Executor{}
	exec.startInteractiveFunc = fakeCopilotInteractiveStart(t, exec)

	err := exec.Setup(t.Context(), &SetupOptions{
		BundleDir:  t.TempDir(),
		TermWidth:  120,
		TermHeight: 30,
	})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if _, writeErr := exec.WriteInput([]byte("help\n")); writeErr != nil {
		t.Fatalf("WriteInput() error = %v", writeErr)
	}

	exec.Teardown()
}

func fakeCopilotInteractiveStart(t *testing.T, exec *Executor) func(context.Context, *SetupOptions) error {
	t.Helper()

	return func(_ context.Context, _ *SetupOptions) error {
		ptmx, err := os.CreateTemp(t.TempDir(), "fake-copilot-pty-*")
		if err != nil {
			return fmt.Errorf("create fake copilot pty: %w", err)
		}

		exec.mu.Lock()
		exec.ptmx = ptmx
		exec.waitDoneCh = make(chan struct{})
		exec.mu.Unlock()

		return nil
	}
}

func installFakeCopilot(t *testing.T, script string) {
	t.Helper()

	binDir := t.TempDir()

	path := filepath.Join(binDir, "copilot")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake copilot: %v", err)
	}

	sep := string(os.PathListSeparator)
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%s%s", binDir, sep, currentPath))
}

func copilotTestJob(prompt string) *client.Job {
	return &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test job"},
		Execution: &client.ExecutionConfig{
			RenderedInstruction: prompt,
		},
	}
}
