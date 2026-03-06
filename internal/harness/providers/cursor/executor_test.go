//go:build unix

package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func TestCursorSetup_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	exec := &Executor{}

	err := exec.Setup(t.Context(), &SetupOptions{})
	if err == nil || !strings.Contains(err.Error(), "cursor-agent CLI not found") {
		t.Fatalf("Setup() err = %v, want binary not found", err)
	}
}

func TestCursorExecute_Success(t *testing.T) {
	installFakeCursorAgent(t, fakeCursorAgentScript)

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	result, err := exec.Execute(t.Context(), cursorTestJob("prompt for cursor", ""))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)
	if output != "cursor output for: prompt for cursor" {
		t.Fatalf("output = %q, want cursor output", output)
	}
}

func TestCursorExecute_Timeout(t *testing.T) {
	installFakeCursorAgent(t, fakeCursorAgentScript)
	t.Setenv("CURSOR_SLEEP", "2")

	exec := &Executor{}
	if err := exec.Setup(t.Context(), &SetupOptions{}); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, cursorTestJob("prompt", ""))
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

func TestCursorExecute_BundleModeQueueRejected(t *testing.T) {
	exec := &Executor{
		opts: SetupOptions{
			BundleDir: "/tmp/some-bundle",
		},
	}

	_, err := exec.Execute(t.Context(), cursorTestJob("prompt", ""))
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

func TestCursorExecute_MergesProjectConfigWithMCP(t *testing.T) {
	installFakeCursorAgent(t, fakeCursorAgentScript)

	workDir := t.TempDir()

	cursorDir := filepath.Join(workDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("mkdir .cursor: %v", err)
	}

	baseConfig := `{"theme":"dark","mcpServers":{"existing":{"type":"http","url":"https://existing.local/mcp"}}}`
	if err := os.WriteFile(filepath.Join(cursorDir, "agent.json"), []byte(baseConfig), 0o600); err != nil {
		t.Fatalf("write base config: %v", err)
	}

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

	result, err := exec.Execute(t.Context(), cursorTestJob("prompt with mcp", workDir))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, _ := result.OutputData["output"].(string)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not merged json: %v\noutput: %s", err, output)
	}

	theme, _ := parsed["theme"].(string)
	if theme != "dark" {
		t.Fatalf("theme = %q, want dark", theme)
	}

	mcpServers, _ := parsed["mcpServers"].(map[string]any)

	linear, ok := mcpServers["linear"].(map[string]any)
	if !ok {
		t.Fatalf("missing linear mcp server in output: %#v", mcpServers)
	}

	url, _ := linear["url"].(string)
	if url != "https://mcp.linear.app/mcp" {
		t.Fatalf("linear.url = %q, want https://mcp.linear.app/mcp", url)
	}
}

func TestCursorNeedsRefresh(t *testing.T) {
	exp := time.Now().Add(10 * time.Minute)
	cfg1 := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok1",
					TokenType:   "bearer",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	cfg2 := &client.RunnerConfigResponse{
		Providers: map[string]client.RunnerProviderConfig{
			"linear": {
				Status: "active",
				Flags:  client.RunnerProviderFlags{MCP: true},
				MCP:    &client.RunnerProviderMCP{URL: "https://mcp.linear.app/mcp"},
				Credential: &client.RunnerProviderCredential{
					AccessToken: "tok2",
					TokenType:   "bearer",
					ExpiresAt:   &exp,
				},
			},
		},
	}

	exec := &Executor{}
	if err := exec.applyRunnerConfig(cfg1); err != nil {
		t.Fatalf("applyRunnerConfig(cfg1) = %v", err)
	}

	if exec.NeedsRefresh(cfg1) {
		t.Fatal("NeedsRefresh(cfg1) = true, want false")
	}

	if !exec.NeedsRefresh(cfg2) {
		t.Fatal("NeedsRefresh(cfg2) = false, want true")
	}
}

func TestCursorSetup_InteractiveBundleMode(t *testing.T) {
	installFakeCursorAgent(t, fakeCursorAgentScript)

	exec := &Executor{}

	err := exec.Setup(t.Context(), &SetupOptions{
		BundleDir:  t.TempDir(),
		TermHeight: 24,
		TermWidth:  80,
	})
	if err != nil {
		t.Fatalf("Setup(bundle mode) error = %v", err)
	}

	exec.Resize(30, 120)

	if _, err := exec.WriteInput([]byte("hello from test\n")); err != nil {
		t.Fatalf("WriteInput() error = %v", err)
	}

	if err := exec.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}

	exec.Teardown()
}

func installFakeCursorAgent(t *testing.T, script string) {
	t.Helper()

	binDir := t.TempDir()

	path := filepath.Join(binDir, "cursor-agent")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake cursor-agent: %v", err)
	}

	sep := string(os.PathListSeparator)
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%s%s", binDir, sep, currentPath))
}

func cursorTestJob(prompt, workDir string) *client.Job {
	exec := &client.ExecutionConfig{
		RenderedInstruction: prompt,
	}
	if strings.TrimSpace(workDir) != "" {
		exec.WorkingDirectory = workDir
	}

	return &client.Job{
		ID:        "job-1",
		QueueID:   "queue-1",
		InputData: map[string]any{"name": "test job"},
		Execution: exec,
	}
}

const fakeCursorAgentScript = `#!/bin/sh
print_mode=0
prompt=""

while [ $# -gt 0 ]; do
  case "$1" in
    --print)
      print_mode=1
      ;;
    --output-format)
      shift
      ;;
    -C)
      shift
      ;;
    *)
      prompt="$1"
      ;;
  esac
  shift
done

if [ "$print_mode" -eq 1 ]; then
  if [ "${CURSOR_SLEEP:-0}" -gt 0 ] 2>/dev/null; then
    sleep "${CURSOR_SLEEP}"
  fi

  if [ "${CURSOR_FAIL:-0}" -eq 1 ] 2>/dev/null; then
    echo "failed run"
    exit 3
  fi

  if [ -n "${CUA_CONFIG_PATH:-}" ] && [ -f "${CUA_CONFIG_PATH}" ]; then
    cat "${CUA_CONFIG_PATH}"
    exit 0
  fi

  echo "cursor output for: ${prompt}"
  exit 0
fi

while IFS= read -r line; do
  echo "$line"
done
`
