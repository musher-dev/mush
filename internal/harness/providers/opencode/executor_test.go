//go:build unix

package opencode

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

func TestOpenCodeSetup_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	exec := &Executor{}

	err := exec.Setup(t.Context(), &SetupOptions{})
	if err == nil || !strings.Contains(err.Error(), "opencode CLI not found") {
		t.Fatalf("Setup() err = %v, want binary not found", err)
	}
}

func TestOpenCodeExecute_ParsesJSONOutput(t *testing.T) {
	installFakeOpenCode(t, `#!/bin/sh
if [ "$1" = "run" ]; then
  if [ -z "$OPENCODE_CONFIG_CONTENT" ]; then
    echo '{"type":"error","error":{"data":{"message":"missing mcp config"}}}'
    exit 1
  fi
  echo '{"type":"text","part":{"text":"first line"}}'
  echo '{"type":"text","part":{"text":"second line"}}'
  exit 0
fi
exit 0
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

	result, err := exec.Execute(t.Context(), testJob("prompt for opencode"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "first line\nsecond line" {
		t.Fatalf("output = %q, want parsed text", output)
	}
}

func TestOpenCodeExecute_FallbackToRawOutput(t *testing.T) {
	installFakeOpenCode(t, `#!/bin/sh
if [ "$1" = "run" ]; then
  echo "plain output from tool"
  exit 0
fi
exit 0
`)

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	result, err := exec.Execute(t.Context(), testJob("prompt for opencode"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "plain output from tool" {
		t.Fatalf("output = %q, want raw fallback output", output)
	}
}

func TestOpenCodeExecute_Timeout(t *testing.T) {
	installFakeOpenCode(t, `#!/bin/sh
if [ "$1" = "run" ]; then
  sleep 2
  exit 0
fi
exit 0
`)

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, testJob("prompt for opencode"))
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

func TestOpenCodeExecute_BundleModeQueueRejected(t *testing.T) {
	exec := &Executor{
		opts: SetupOptions{
			BundleDir: "/tmp/some-bundle",
		},
	}

	_, err := exec.Execute(t.Context(), testJob("prompt"))
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

func installFakeOpenCode(t *testing.T, script string) {
	t.Helper()

	binDir := t.TempDir()

	path := filepath.Join(binDir, "opencode")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}

	sep := string(os.PathListSeparator)
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%s%s", binDir, sep, currentPath))
}

func testJob(prompt string) *client.Job {
	return &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test job"},
		Execution: &client.ExecutionConfig{
			RenderedInstruction: prompt,
		},
	}
}
