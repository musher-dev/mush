package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

func TestClient_ValidateKey(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid key",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "no content is valid",
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
			errMsg:     "invalid or expired API key",
		},
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			wantErr:    true,
			errMsg:     "API key does not have runner permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify auth header
				auth := r.Header.Get("Authorization")
				if auth != "Bearer test-key" {
					t.Errorf("Authorization header = %q, want %q", auth, "Bearer test-key")
				}

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			c := New(server.URL, "test-key")
			_, err := c.ValidateKey(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if err.Error() != tt.errMsg {
					t.Errorf("ValidateKey() error = %q, want %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestClient_ClaimJob(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   interface{}
		wantJob    bool
		wantErr    bool
	}{
		{
			name:       "job available",
			statusCode: http.StatusOK,
			response: JobClaimResponse{
				Job: Job{
					ID:            "job-123",
					JobType:       "webhook",
					QueueID:       "queue-123",
					Priority:      "normal",
					Status:        "queued",
					AttemptNumber: 1,
					MaxAttempts:   3,
				},
			},
			wantJob: true,
			wantErr: false,
		},
		{
			name:       "no content",
			statusCode: http.StatusNoContent,
			wantJob:    false,
			wantErr:    false,
		},
		{
			name:       "null response",
			statusCode: http.StatusOK,
			response:   nil,
			wantJob:    false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/runner/jobs:claim" {
					t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs:claim")
				}

				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					json.NewEncoder(w).Encode(tt.response)
				} else if tt.statusCode == http.StatusOK {
					w.Write([]byte("null"))
				}
			}))
			defer server.Close()

			c := New(server.URL, "test-key")
			job, err := c.ClaimJob(context.Background(), "habitat-123", "", 30)

			if (err != nil) != tt.wantErr {
				t.Errorf("ClaimJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantJob && job == nil {
				t.Error("ClaimJob() job should not be nil")
			}
			if !tt.wantJob && job != nil {
				t.Error("ClaimJob() job should be nil")
			}
		})
	}
}

func TestClient_ListQueues(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "queues available",
			statusCode: http.StatusOK,
			response:   `{"data":[{"id":"queue-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`,
			wantCount:  1,
			wantErr:    false,
		},
		{
			name:       "empty queue list",
			statusCode: http.StatusOK,
			response:   `{"data":[]}`,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name:       "error response",
			statusCode: http.StatusForbidden,
			response:   `{"detail":"forbidden"}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/queues" {
					t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/queues")
				}
				if r.URL.Query().Get("status") != "active" {
					t.Errorf("status query = %q, want active", r.URL.Query().Get("status"))
				}
				if r.URL.Query().Get("habitat_id") != "hab-1" {
					t.Errorf("habitat_id query = %q, want hab-1", r.URL.Query().Get("habitat_id"))
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			c := New(server.URL, "test-key")
			queues, err := c.ListQueues(context.Background(), "hab-1")
			if (err != nil) != tt.wantErr {
				t.Errorf("ListQueues() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(queues) != tt.wantCount {
				t.Errorf("ListQueues() count = %d, want %d", len(queues), tt.wantCount)
			}
		})
	}
}

func TestClient_HeartbeatJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/jobs/job-123:heartbeat" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs/job-123:heartbeat")
		}
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}

		// Return updated job
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Job{
			ID:            "job-123",
			JobType:       "webhook",
			QueueID:       "queue-123",
			Priority:      "normal",
			Status:        "claimed",
			AttemptNumber: 1,
			MaxAttempts:   3,
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	job, err := c.HeartbeatJob(context.Background(), "job-123")
	if err != nil {
		t.Errorf("HeartbeatJob() error = %v", err)
	}
	if job == nil {
		t.Fatal("HeartbeatJob() job should not be nil")
	}
	if job.ID != "job-123" {
		t.Errorf("HeartbeatJob() id = %q, want %q", job.ID, "job-123")
	}
}

func TestClient_CompleteJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/jobs/job-123:complete" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs/job-123:complete")
		}

		var req JobCompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.CompleteJob(context.Background(), "job-123", map[string]interface{}{"result": "success"})
	if err != nil {
		t.Errorf("CompleteJob() error = %v", err)
	}
}

func TestClient_FailJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/jobs/job-123:fail" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs/job-123:fail")
		}

		var req JobFailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.ErrorCode != "execution_error" {
			t.Errorf("errorCode = %q, want %q", req.ErrorCode, "execution_error")
		}
		if req.ErrorMessage != "test error" {
			t.Errorf("errorMessage = %q, want %q", req.ErrorMessage, "test error")
		}
		if !req.ShouldRetry {
			t.Error("shouldRetry should be true")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.FailJob(context.Background(), "job-123", "execution_error", "test error", true)
	if err != nil {
		t.Errorf("FailJob() error = %v", err)
	}
}

func TestClient_RegisterLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/links:register" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/links:register")
		}

		var req RegisterLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.InstanceID != "instance-1" {
			t.Errorf("instanceId = %q, want %q", req.InstanceID, "instance-1")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(RegisterLinkResponse{
			LinkID:              "link-123",
			WorkerID:            "worker-456",
			HeartbeatIntervalMs: 30000,
			HeartbeatDeadlineAt: time.Now().Add(time.Minute),
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	resp, err := c.RegisterLink(context.Background(), &RegisterLinkRequest{
		InstanceID: "instance-1",
		LinkType:   "agent",
	})
	if err != nil {
		t.Errorf("RegisterLink() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RegisterLink() response should not be nil")
	}
	if resp.LinkID != "link-123" {
		t.Errorf("linkId = %q, want %q", resp.LinkID, "link-123")
	}
}

func TestClient_HeartbeatLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/links/link-123:heartbeat" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/links/link-123:heartbeat")
		}

		var req LinkHeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.CurrentJobID != "job-123" {
			t.Errorf("current job id = %q, want %q", req.CurrentJobID, "job-123")
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LinkHeartbeatResponse{
			Status:              "active",
			HeartbeatDeadlineAt: time.Now().Add(time.Minute),
		})
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	resp, err := c.HeartbeatLink(context.Background(), "link-123", "job-123")
	if err != nil {
		t.Errorf("HeartbeatLink() error = %v", err)
	}
	if resp == nil {
		t.Fatal("HeartbeatLink() response should not be nil")
	}
	if resp.Status != "active" {
		t.Errorf("status = %q, want %q", resp.Status, "active")
	}
}

func TestClient_DeregisterLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/links/link-123:deregister" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/links/link-123:deregister")
		}

		var req DeregisterLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Reason != "graceful_shutdown" {
			t.Errorf("reason = %q, want %q", req.Reason, "graceful_shutdown")
		}
		if req.JobsCompleted != 5 {
			t.Errorf("jobs completed = %d, want %d", req.JobsCompleted, 5)
		}
		if req.JobsFailed != 1 {
			t.Errorf("jobs failed = %d, want %d", req.JobsFailed, 1)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "test-key")
	err := c.DeregisterLink(context.Background(), "link-123", DeregisterLinkRequest{
		Reason:        "graceful_shutdown",
		JobsCompleted: 5,
		JobsFailed:    1,
	})
	if err != nil {
		t.Errorf("DeregisterLink() error = %v", err)
	}
}

func TestClient_ReleaseJob(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/runner/jobs/job-123:release" {
				t.Errorf("path = %q, want %q", r.URL.Path, "/api/v1/runner/jobs/job-123:release")
			}
			if r.Method != "POST" {
				t.Errorf("method = %q, want POST", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := New(server.URL, "test-key")
		err := c.ReleaseJob(context.Background(), "job-123")
		if err != nil {
			t.Errorf("ReleaseJob() error = %v", err)
		}
	})

	t.Run("error on non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"detail":"conflict"}`))
		}))
		defer server.Close()

		c := New(server.URL, "test-key")
		err := c.ReleaseJob(context.Background(), "job-123")
		if err == nil {
			t.Error("ReleaseJob() should return error on 409")
		}
	})
}

func TestJob_Fields(t *testing.T) {
	job := Job{
		ID:            "job-123",
		JobType:       "webhook",
		QueueID:       "queue-1",
		Priority:      "normal",
		Status:        "running",
		InputData:     map[string]interface{}{"pr": 42},
		AttemptNumber: 1,
		MaxAttempts:   3,
	}

	if job.ID != "job-123" {
		t.Errorf("ID = %q, want %q", job.ID, "job-123")
	}
	if job.JobType != "webhook" {
		t.Errorf("JobType = %q, want %q", job.JobType, "webhook")
	}
	if job.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", job.AttemptNumber)
	}
	if job.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", job.MaxAttempts)
	}
}
