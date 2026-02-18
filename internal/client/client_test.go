package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newMockClient(t *testing.T, fn roundTripFunc) *Client {
	t.Helper()

	hc := &http.Client{Transport: fn}

	return NewWithHTTPClient("https://api.test", "test-key", hc)
}

func TestNew(t *testing.T) {
	baseURL := "https://custom.api.com"
	apiKey := "test-api-key"
	c := New(baseURL, apiKey)

	if c.apiKey != apiKey {
		t.Errorf("apiKey = %q, want %q", c.apiKey, apiKey)
	}

	if c.baseURL != baseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, baseURL)
	}

	if c.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestClientValidateKey(t *testing.T) {
	identityJSON := `{"credentialType":"service_account","credentialName":"my-ci-runner","workspaceId":"ws-456","workspaceName":"Acme Corp"}`

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{name: "valid key", statusCode: http.StatusOK, body: identityJSON},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantErr: "invalid or expired API key"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantErr: "API key does not have runner permissions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/v1/runner/me" {
					t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v1/runner/me")
				}

				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Fatalf("Authorization = %q, want %q", got, "Bearer test-key")
				}

				return jsonResponse(tt.statusCode, tt.body), nil
			})

			identity, err := c.ValidateKey(t.Context())
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ValidateKey() error = %v", err)
			}

			if identity.WorkspaceName != "Acme Corp" {
				t.Fatalf("WorkspaceName = %q, want %q", identity.WorkspaceName, "Acme Corp")
			}
		})
	}
}

func TestClientClaimJob(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantJob    bool
	}{
		{name: "job available", statusCode: http.StatusOK, body: `{"job":{"id":"job-123","queueId":"queue-123","priority":"normal","status":"queued","attemptNumber":1,"maxAttempts":3}}`, wantJob: true},
		{name: "no content", statusCode: http.StatusNoContent, body: "", wantJob: false},
		{name: "null response", statusCode: http.StatusOK, body: "null", wantJob: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/v1/runner/jobs:claim" {
					t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs:claim")
				}

				if r.URL.Query().Get("wait_timeout_seconds") != "30" {
					t.Fatalf("wait_timeout_seconds = %q, want 30", r.URL.Query().Get("wait_timeout_seconds"))
				}

				return jsonResponse(tt.statusCode, tt.body), nil
			})

			job, err := c.ClaimJob(t.Context(), "habitat-123", "", 30)
			if err != nil {
				t.Fatalf("ClaimJob() error = %v", err)
			}

			if tt.wantJob && job == nil {
				t.Fatal("expected job")
			}

			if !tt.wantJob && job != nil {
				t.Fatal("expected nil job")
			}
		})
	}
}

func TestClientListQueues(t *testing.T) {
	c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/queues" {
			t.Fatalf("path = %q, want /api/v1/queues", r.URL.Path)
		}

		if got := r.URL.Query().Get("status"); got != "active" {
			t.Fatalf("status = %q, want active", got)
		}

		if got := r.URL.Query().Get("habitat_id"); got != "hab-1" {
			t.Fatalf("habitat_id = %q, want hab-1", got)
		}

		return jsonResponse(http.StatusOK, `{"data":[{"id":"queue-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`), nil
	})

	queues, err := c.ListQueues(t.Context(), "hab-1")
	if err != nil {
		t.Fatalf("ListQueues() error = %v", err)
	}

	if len(queues) != 1 || queues[0].ID != "queue-1" {
		t.Fatalf("unexpected queues: %#v", queues)
	}
}

func TestClientGetRunnerConfig(t *testing.T) {
	c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/runner/config" {
			t.Fatalf("path = %q, want /api/v1/runner/config", r.URL.Path)
		}

		return jsonResponse(http.StatusOK, `{"configVersion":"1","workspaceId":"ws-123","generatedAt":"2026-02-13T12:00:00Z","refreshAfterSeconds":300,"providers":{"linear":{"status":"active","credential":{"accessToken":"tok_123"},"flags":{"mcp":true}}}}`), nil
	})

	cfg, err := c.GetRunnerConfig(t.Context())
	if err != nil {
		t.Fatalf("GetRunnerConfig() error = %v", err)
	}

	if cfg.WorkspaceID != "ws-123" || cfg.RefreshAfterSeconds != 300 {
		t.Fatalf("unexpected config: %#v", cfg)
	}

	if !cfg.Providers["linear"].Flags.MCP {
		t.Fatalf("expected linear MCP enabled")
	}
}

func TestClientJobLifecycleEndpoints(t *testing.T) {
	c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/runner/jobs/job-123:start":
			return jsonResponse(http.StatusOK, `{"id":"job-123"}`), nil
		case "/api/v1/runner/jobs/job-123:heartbeat":
			return jsonResponse(http.StatusOK, `{"id":"job-123"}`), nil
		case "/api/v1/runner/jobs/job-123:complete":
			return jsonResponse(http.StatusOK, `{}`), nil
		case "/api/v1/runner/jobs/job-123:fail":
			var req JobFailRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode fail request: %v", err)
			}

			if req.ErrorCode != "execution_error" || !req.ShouldRetry {
				t.Fatalf("unexpected fail request: %#v", req)
			}

			return jsonResponse(http.StatusOK, `{}`), nil
		case "/api/v1/runner/jobs/job-123:release":
			return jsonResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil //nolint:nilnil // unreachable after t.Fatalf
		}
	})

	if _, err := c.StartJob(t.Context(), "job-123"); err != nil {
		t.Fatalf("StartJob() error = %v", err)
	}

	if _, err := c.HeartbeatJob(t.Context(), "job-123"); err != nil {
		t.Fatalf("HeartbeatJob() error = %v", err)
	}

	if err := c.CompleteJob(t.Context(), "job-123", map[string]any{"result": "success"}); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}

	if err := c.FailJob(t.Context(), "job-123", "execution_error", "test error", true); err != nil {
		t.Fatalf("FailJob() error = %v", err)
	}

	if err := c.ReleaseJob(t.Context(), "job-123"); err != nil {
		t.Fatalf("ReleaseJob() error = %v", err)
	}
}

func TestClientLinkLifecycleEndpoints(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/runner/links:register":
			return jsonResponse(http.StatusCreated, `{"linkId":"link-123","workerId":"worker-456","heartbeatDeadlineAt":"`+now+`","heartbeatIntervalMs":30000}`), nil
		case "/api/v1/runner/links/link-123:heartbeat":
			return jsonResponse(http.StatusOK, `{"status":"active","heartbeatDeadlineAt":"`+now+`"}`), nil
		case "/api/v1/runner/links/link-123:deregister":
			return jsonResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil //nolint:nilnil // unreachable after t.Fatalf
		}
	})

	resp, err := c.RegisterLink(t.Context(), &RegisterLinkRequest{InstanceID: "instance-1", LinkType: "harness"})
	if err != nil || resp.LinkID != "link-123" {
		t.Fatalf("RegisterLink() resp=%#v err=%v", resp, err)
	}

	if _, err := c.HeartbeatLink(t.Context(), "link-123", "job-123"); err != nil {
		t.Fatalf("HeartbeatLink() error = %v", err)
	}

	if err := c.DeregisterLink(t.Context(), "link-123", DeregisterLinkRequest{Reason: "graceful_shutdown", JobsCompleted: 5, JobsFailed: 1}); err != nil {
		t.Fatalf("DeregisterLink() error = %v", err)
	}
}

func TestJobFieldsAndHelpers(t *testing.T) {
	job := Job{
		ID:            "job-123",
		JobType:       "webhook",
		QueueID:       "queue-1",
		Priority:      "normal",
		Status:        "running",
		InputData:     map[string]any{"title": "Fix bug"},
		AttemptNumber: 1,
		MaxAttempts:   3,
		Execution:     &ExecutionConfig{HarnessType: "bash", RenderedInstruction: "echo hi"},
	}

	if job.GetHarnessType() != "bash" {
		t.Fatalf("GetHarnessType = %q, want bash", job.GetHarnessType())
	}

	if got := job.GetRenderedInstruction(); got != "echo hi" {
		t.Fatalf("GetRenderedInstruction = %q, want %q", got, "echo hi")
	}

	if got := job.GetDisplayName(); got != "Fix bug" {
		t.Fatalf("GetDisplayName = %q, want %q", got, "Fix bug")
	}
}

func TestClaimJobSendsJSONBody(t *testing.T) {
	c := newMockClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/runner/jobs:claim" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if !bytes.Contains(payload, []byte(`"leaseDurationMs":45000`)) {
			t.Fatalf("body missing leaseDurationMs: %s", string(payload))
		}

		return jsonResponse(http.StatusNoContent, ""), nil
	})

	if _, err := c.ClaimJob(t.Context(), "hab-1", "", 30); err != nil {
		t.Fatalf("ClaimJob() error = %v", err)
	}
}
