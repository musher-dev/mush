package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

// linkMockServer creates a test server that handles all API calls needed by
// the link command up to the dry-run / TTY check point.
func linkMockServer(t *testing.T) *httptest.Server {
	return linkMockServerWithConfig(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`)
}

func linkMockServerWithConfig(t *testing.T, runnerConfig string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/runner/me" && r.Method == "GET":
			_, _ = w.Write([]byte(`{"credentialType":"service_account","credentialId":"cred-1","credentialName":"test-sa","workerId":"sa_xxx","workspaceId":"ws-1","workspaceName":"Test Workspace"}`))
		case r.URL.Path == "/api/v1/runner/config" && r.Method == "GET":
			_, _ = w.Write([]byte(runnerConfig))
		case r.URL.Path == "/api/v1/runner/habitats" && r.Method == "GET":
			_, _ = w.Write([]byte(`[{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"}]`))
		case r.URL.Path == "/api/v1/queues" && r.Method == "GET":
			_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/runner/queues/") && strings.HasSuffix(r.URL.Path, "/instruction-availability"):
			_, _ = w.Write([]byte(`{"queueId":"q-1","hasActiveInstruction":true}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func TestLinkDryRun_SucceedsWithoutTTY(t *testing.T) {
	server := linkMockServer(t)
	defer server.Close()

	// Non-TTY terminal
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	t.Setenv("MUSHER_API_KEY", "test-key")
	t.Setenv("MUSH_API_URL", server.URL)

	cmd := newLinkCmd()
	cmd.SetArgs([]string{"--dry-run", "--habitat", "local", "--queue-id", "q-1"})
	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("dry-run should succeed without TTY, got error: %v", err)
	}
}

func TestLinkNoDryRun_RequiresTTY(t *testing.T) {
	server := linkMockServer(t)
	defer server.Close()

	// Non-TTY terminal
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	t.Setenv("MUSHER_API_KEY", "test-key")
	t.Setenv("MUSH_API_URL", server.URL)

	cmd := newLinkCmd()
	cmd.SetArgs([]string{"--habitat", "local", "--queue-id", "q-1"})
	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected TTY error for non-dry-run without terminal")
	}
	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("error code = %d, want %d (ExitUsage)", cliErr.Code, clierrors.ExitUsage)
	}
	if !strings.Contains(cliErr.Message, "TTY") {
		t.Fatalf("error message = %q, want to contain 'TTY'", cliErr.Message)
	}
}

func TestLinkDryRun_PrintsMCPServers(t *testing.T) {
	server := linkMockServerWithConfig(t, `{
		"configVersion":"1",
		"workspaceId":"ws-1",
		"generatedAt":"2026-02-13T12:00:00Z",
		"refreshAfterSeconds":300,
		"providers":{
			"linear":{
				"status":"active",
				"credential":{"accessToken":"tok","tokenType":"bearer","expiresAt":"2099-12-31T23:59:59Z"},
				"flags":{"mcp":true},
				"mcp":{"url":"https://mcp.linear.app/mcp","transport":"streamable-http"}
			}
		}
	}`)
	defer server.Close()

	var outBuf bytes.Buffer
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&outBuf, io.Discard, term)
	out.NoInput = true

	t.Setenv("MUSHER_API_KEY", "test-key")
	t.Setenv("MUSH_API_URL", server.URL)

	cmd := newLinkCmd()
	cmd.SetArgs([]string{"--dry-run", "--habitat", "local", "--queue-id", "q-1", "--agent", "claude"})
	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should succeed, got error: %v", err)
	}

	got := outBuf.String()
	if !strings.Contains(got, "MCP servers: linear") {
		t.Fatalf("output = %q, expected MCP servers line", got)
	}
}

func TestLinkDryRun_BashAgentOmitsMCPServers(t *testing.T) {
	server := linkMockServer(t)
	defer server.Close()

	var outBuf bytes.Buffer
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&outBuf, io.Discard, term)
	out.NoInput = true

	t.Setenv("MUSHER_API_KEY", "test-key")
	t.Setenv("MUSH_API_URL", server.URL)

	cmd := newLinkCmd()
	cmd.SetArgs([]string{"--dry-run", "--habitat", "local", "--queue-id", "q-1", "--agent", "bash"})
	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should succeed, got error: %v", err)
	}

	got := outBuf.String()
	if strings.Contains(got, "MCP servers:") {
		t.Fatalf("output = %q, expected no MCP servers line for bash-only agent", got)
	}
}

func TestResolveQueue_WithFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queues" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	queue, err := resolveQueue(t.Context(), c, "hab-1", "q-1", out)
	if err != nil {
		t.Fatalf("resolveQueue returned error: %v", err)
	}
	if queue.ID != "q-1" {
		t.Fatalf("queue.ID = %q, want q-1", queue.ID)
	}
}

func TestResolveQueue_WithInvalidFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	_, err := resolveQueue(t.Context(), c, "hab-1", "q-missing", out)
	if err == nil {
		t.Fatal("expected error for missing queue flag")
	}
}

func TestResolveQueue_NoQueuesForHabitat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	_, err := resolveQueue(t.Context(), c, "hab-1", "", out)
	if err == nil {
		t.Fatal("expected no queues error")
	}
}

func TestResolveQueue_AutoSelectSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	queue, err := resolveQueue(t.Context(), c, "hab-1", "", out)
	if err != nil {
		t.Fatalf("resolveQueue returned error: %v", err)
	}
	if queue.ID != "q-1" {
		t.Fatalf("queue.ID = %q, want q-1", queue.ID)
	}
}

func TestResolveQueue_NoInputMultipleQueues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[` +
			`{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"},` +
			`{"id":"q-2","slug":"other","name":"Other","status":"active","habitatId":"hab-1"}` +
			`]}`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	_, err := resolveQueue(t.Context(), c, "hab-1", "", out)
	if err == nil {
		t.Fatal("expected error for multiple queues in no-input mode")
	}
	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Message != "Queue required" {
		t.Fatalf("error message = %q, want %q", cliErr.Message, "Queue required")
	}
}

func TestResolveHabitatID_AutoSelectSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"}]`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	id, err := resolveHabitatID(t.Context(), c, "", out)
	if err != nil {
		t.Fatalf("resolveHabitatID returned error: %v", err)
	}
	if id != "hab-1" {
		t.Fatalf("habitat ID = %q, want hab-1", id)
	}
}

func TestResolveHabitatID_NoInputMultipleHabitats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[` +
			`{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"},` +
			`{"id":"hab-2","slug":"staging","name":"Staging","status":"offline","habitatType":"cloud"}` +
			`]`))
	}))
	defer server.Close()

	c := client.New(server.URL, "test-key")
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	_, err := resolveHabitatID(t.Context(), c, "", out)
	if err == nil {
		t.Fatal("expected error for multiple habitats in no-input mode")
	}
	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Message != "Habitat required" {
		t.Fatalf("error message = %q, want %q", cliErr.Message, "Habitat required")
	}
}
