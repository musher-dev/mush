package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

type workerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f workerRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func workerJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func workerMockClient(t *testing.T, runnerConfig string) *client.Client {
	t.Helper()

	hc := &http.Client{Transport: workerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/api/v1/runner/me" && r.Method == http.MethodGet:
			return workerJSONResponse(http.StatusOK, `{"credentialType":"service_account","credentialId":"cred-1","credentialName":"test-sa","runnerId":"sa_xxx","workspaceId":"ws-1","workspaceName":"Test Workspace"}`), nil
		case r.URL.Path == "/api/v1/runner/config" && r.Method == http.MethodGet:
			return workerJSONResponse(http.StatusOK, runnerConfig), nil
		case r.URL.Path == "/api/v1/runner/habitats" && r.Method == http.MethodGet:
			return workerJSONResponse(http.StatusOK, `[{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"}]`), nil
		case r.URL.Path == "/api/v1/queues" && r.Method == http.MethodGet:
			return workerJSONResponse(http.StatusOK, `{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`), nil
		case strings.HasPrefix(r.URL.Path, "/api/v1/runner/queues/") && strings.HasSuffix(r.URL.Path, "/instruction-availability"):
			return workerJSONResponse(http.StatusOK, `{"queueId":"q-1","hasActiveInstruction":true}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil //nolint:nilnil // unreachable after t.Fatalf
		}
	})}

	return client.NewWithHTTPClient("https://api.test", "test-key", hc)
}

func withMockAPIClient(t *testing.T, c *client.Client) {
	t.Helper()

	prev := apiClientFactory
	apiClientFactory = func() (auth.CredentialSource, *client.Client, error) {
		return auth.CredentialSource("env"), c, nil
	}

	t.Cleanup(func() {
		apiClientFactory = prev
	})
}

func TestWorkerStartDryRunSucceedsWithoutTTY(t *testing.T) {
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--dry-run", "--habitat", "local", "--queue", "q-1"})

	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should succeed without TTY, got error: %v", err)
	}
}

func TestWorkerStartNoDryRunRequiresTTY(t *testing.T) {
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--habitat", "local", "--queue", "q-1"})

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

func TestWorkerStartDryRunPrintsMCPServers(t *testing.T) {
	var outBuf bytes.Buffer

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&outBuf, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{"linear":{"status":"active","credential":{"accessToken":"tok","tokenType":"bearer","expiresAt":"2099-12-31T23:59:59Z"},"flags":{"mcp":true},"mcp":{"url":"https://mcp.linear.app/mcp","transport":"streamable-http"}}}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--dry-run", "--habitat", "local", "--queue", "q-1", "--harness", "claude"})

	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should succeed, got error: %v", err)
	}

	if got := outBuf.String(); !strings.Contains(got, "MCP servers: linear") {
		t.Fatalf("output = %q, expected MCP servers line", got)
	}
}

func TestWorkerStartDryRunBashHarnessOmitsMCPServers(t *testing.T) {
	var outBuf bytes.Buffer

	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(&outBuf, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--dry-run", "--habitat", "local", "--queue", "q-1", "--harness", "bash"})

	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should succeed, got error: %v", err)
	}

	if got := outBuf.String(); strings.Contains(got, "MCP servers:") {
		t.Fatalf("output = %q, expected no MCP servers line for bash-only harness", got)
	}
}

func TestWorkerStartLegacyAgentFlagRejected(t *testing.T) {
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--dry-run", "--habitat", "local", "--queue", "q-1", "--agent", "claude"})

	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for legacy --agent flag")
	}

	if !strings.Contains(err.Error(), "unknown flag: --agent") {
		t.Fatalf("error = %q, want unknown flag --agent", err.Error())
	}
}

func TestWorkerStartBundleFlagAccepted(t *testing.T) {
	cmd := newWorkerCmd()

	// Verify the --bundle flag exists on the start subcommand.
	startCmd, _, err := cmd.Find([]string{"start"})
	if err != nil {
		t.Fatalf("failed to find start subcommand: %v", err)
	}

	f := startCmd.Flags().Lookup("bundle")
	if f == nil {
		t.Fatal("expected --bundle flag on worker start command")
	}

	if f.DefValue != "" {
		t.Fatalf("--bundle default = %q, want empty", f.DefValue)
	}
}

func TestWorkerStartBundleFlagInvalidRef(t *testing.T) {
	term := &terminal.Info{IsTTY: false}
	out := output.NewWriter(io.Discard, io.Discard, term)
	out.NoInput = true

	withMockAPIClient(t, workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`))

	cmd := newWorkerCmd()
	cmd.SetArgs([]string{"start", "--dry-run", "--habitat", "local", "--queue", "q-1", "--bundle", ":"})

	ctx := out.WithContext(t.Context())
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid bundle ref")
	}

	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}

	if cliErr.Code != clierrors.ExitUsage {
		t.Fatalf("error code = %d, want %d (ExitUsage)", cliErr.Code, clierrors.ExitUsage)
	}
}

func TestResolveQueueAndHabitatNoInputSelection(t *testing.T) {
	c := workerMockClient(t, `{"configVersion":"1","workspaceId":"ws-1","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{}}`)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	habitatID, err := resolveHabitatID(t.Context(), c, "", out)
	if err != nil {
		t.Fatalf("resolveHabitatID() error = %v", err)
	}

	if habitatID != "hab-1" {
		t.Fatalf("resolveHabitatID() = %q, want hab-1", habitatID)
	}

	queue, err := resolveQueue(t.Context(), c, "hab-1", "", out)
	if err != nil {
		t.Fatalf("resolveQueue() error = %v", err)
	}

	if queue.ID != "q-1" {
		t.Fatalf("resolveQueue().ID = %q, want q-1", queue.ID)
	}
}
