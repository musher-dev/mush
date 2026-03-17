// Package client provides the API client for communicating with the Musher platform.
//
// The client handles authentication and provides methods for:
//   - Validating runner API keys
//   - Claiming jobs from queues
//   - Reporting job completion/failure
//   - Sending job heartbeats
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 60 * time.Second
	// DefaultLeaseDurationMs is the default job lease duration (45s to allow margin over 30s heartbeat).
	DefaultLeaseDurationMs = 45000
)

// Client is the Musher API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// HTTPStatusError is returned when an API call receives a non-success HTTP status.
type HTTPStatusError struct {
	Operation string
	Status    int
	RequestID string
	TraceID   string
}

func (e *HTTPStatusError) Error() string {
	var extras []string
	if e.RequestID != "" {
		extras = append(extras, "request_id="+e.RequestID)
	}

	if e.TraceID != "" {
		extras = append(extras, "trace_id="+e.TraceID)
	}

	if len(extras) == 0 {
		return fmt.Sprintf("%s failed with status %d", e.Operation, e.Status)
	}

	return fmt.Sprintf("%s failed with status %d (%s)", e.Operation, e.Status, strings.Join(extras, ", "))
}

// RequestIDValue returns the request correlation ID when available.
func (e *HTTPStatusError) RequestIDValue() string { return e.RequestID }

// TraceIDValue returns the distributed trace ID when available.
func (e *HTTPStatusError) TraceIDValue() string { return e.TraceID }

// RequestError represents a transport-level request failure.
type RequestError struct {
	Operation string
	RequestID string
	Cause     error
}

func (e *RequestError) Error() string {
	if e.RequestID == "" {
		return fmt.Sprintf("%s: %v", e.Operation, e.Cause)
	}

	return fmt.Sprintf("%s (request_id=%s): %v", e.Operation, e.RequestID, e.Cause)
}

func (e *RequestError) Unwrap() error { return e.Cause }

// RequestIDValue returns the request correlation ID when available.
func (e *RequestError) RequestIDValue() string { return e.RequestID }

// Identity represents the authenticated runner identity.
type Identity struct {
	CredentialType   string `json:"credentialType"`
	CredentialID     string `json:"credentialId"`
	CredentialName   string `json:"credentialName"`
	RunnerID         string `json:"runnerId"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
}

// UserProfile represents the authenticated user profile.
type UserProfile struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	FullName string `json:"fullName"`
}

// UnmarshalJSON accepts both organization-scoped and legacy workspace-scoped payloads.
func (i *Identity) UnmarshalJSON(data []byte) error {
	type identityAlias struct {
		CredentialType   string `json:"credentialType"`
		CredentialID     string `json:"credentialId"`
		CredentialName   string `json:"credentialName"`
		RunnerID         string `json:"runnerId"`
		OrganizationID   string `json:"organizationId"`
		OrganizationName string `json:"organizationName"`
		WorkspaceID      string `json:"workspaceId"`
		WorkspaceName    string `json:"workspaceName"`
	}

	var aux identityAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("unmarshal identity: %w", err)
	}

	i.CredentialType = aux.CredentialType
	i.CredentialID = aux.CredentialID
	i.CredentialName = aux.CredentialName
	i.RunnerID = aux.RunnerID
	i.OrganizationID = firstNonEmpty(aux.OrganizationID, aux.WorkspaceID)
	i.OrganizationName = firstNonEmpty(aux.OrganizationName, aux.WorkspaceName)

	return nil
}

// ResponseMeta contains correlation metadata from an API response.
type ResponseMeta struct {
	RequestID string `json:"requestId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
}

// RunnerConfigResponse is the generic runner configuration payload.
type RunnerConfigResponse struct {
	ConfigVersion       string                          `json:"configVersion"`
	OrganizationID      string                          `json:"organizationId"`
	GeneratedAt         time.Time                       `json:"generatedAt"`
	RefreshAfterSeconds int                             `json:"refreshAfterSeconds"`
	Providers           map[string]RunnerProviderConfig `json:"providers"`
	Errors              []RunnerConfigError             `json:"errors,omitempty"`
}

// UnmarshalJSON accepts both organization-scoped and legacy workspace-scoped payloads.
func (r *RunnerConfigResponse) UnmarshalJSON(data []byte) error {
	type runnerConfigAlias struct {
		ConfigVersion       string                          `json:"configVersion"`
		OrganizationID      string                          `json:"organizationId"`
		WorkspaceID         string                          `json:"workspaceId"`
		GeneratedAt         time.Time                       `json:"generatedAt"`
		RefreshAfterSeconds int                             `json:"refreshAfterSeconds"`
		Providers           map[string]RunnerProviderConfig `json:"providers"`
		Errors              []RunnerConfigError             `json:"errors,omitempty"`
	}

	var aux runnerConfigAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("unmarshal runner config: %w", err)
	}

	r.ConfigVersion = aux.ConfigVersion
	r.OrganizationID = firstNonEmpty(aux.OrganizationID, aux.WorkspaceID)
	r.GeneratedAt = aux.GeneratedAt
	r.RefreshAfterSeconds = aux.RefreshAfterSeconds
	r.Providers = aux.Providers
	r.Errors = aux.Errors

	return nil
}

// RunnerProviderConfig contains provider-specific runner configuration.
type RunnerProviderConfig struct {
	Status     string                    `json:"status"`
	Credential *RunnerProviderCredential `json:"credential,omitempty"`
	Flags      RunnerProviderFlags       `json:"flags,omitempty"`
	MCP        *RunnerProviderMCP        `json:"mcp,omitempty"`
	Metadata   map[string]any            `json:"metadata,omitempty"`
}

// RunnerProviderCredential contains provider credentials for runtime use.
type RunnerProviderCredential struct {
	AccessToken string     `json:"accessToken"`
	TokenType   string     `json:"tokenType,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// RunnerProviderFlags contains feature flags for a provider.
type RunnerProviderFlags struct {
	MCP bool `json:"mcp"`
}

// RunnerProviderMCP contains MCP endpoint metadata for a provider.
type RunnerProviderMCP struct {
	URL       string `json:"url"`
	Transport string `json:"transport,omitempty"`
}

// RunnerConfigError contains partial error details for provider config resolution.
type RunnerConfigError struct {
	Provider string `json:"provider"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// JobClaimRequest is the request body for claiming a job.
type JobClaimRequest struct {
	QueueID         string `json:"queueId,omitempty"`
	HabitatID       string `json:"habitatId,omitempty"`
	LeaseDurationMs int    `json:"leaseDurationMs"`
}

// JobCompleteRequest is the request body for completing a job.
type JobCompleteRequest struct {
	OutputData map[string]any `json:"outputData,omitempty"`
}

// JobFailRequest is the request body for failing a job.
type JobFailRequest struct {
	ErrorCode    string         `json:"errorCode,omitempty"`
	ErrorMessage string         `json:"errorMessage,omitempty"`
	ErrorDetails map[string]any `json:"errorDetails,omitempty"`
	ShouldRetry  bool           `json:"shouldRetry"`
}

// RegisterWorkerRequest is the request body for registering a worker.
type RegisterWorkerRequest struct {
	InstanceID     string         `json:"instanceId"`
	HabitatID      string         `json:"habitatId"`
	Name           string         `json:"name,omitempty"`
	WorkerType     string         `json:"workerType"`
	ClientVersion  string         `json:"clientVersion,omitempty"`
	ClientMetadata map[string]any `json:"clientMetadata,omitempty"`
}

// RegisterWorkerResponse is the response from registering a worker.
type RegisterWorkerResponse struct {
	WorkerID            string    `json:"workerId"`
	RunnerID            string    `json:"runnerId"`
	HeartbeatDeadlineAt time.Time `json:"heartbeatDeadlineAt"`
	HeartbeatIntervalMs int       `json:"heartbeatIntervalMs"`
}

// WorkerHeartbeatRequest is the request body for worker heartbeat.
type WorkerHeartbeatRequest struct {
	CurrentJobID string `json:"currentJobId,omitempty"`
}

// WorkerHeartbeatResponse is the response from worker heartbeat.
type WorkerHeartbeatResponse struct {
	Status              string    `json:"status"`
	HeartbeatDeadlineAt time.Time `json:"heartbeatDeadlineAt"`
}

// DeregisterWorkerRequest is the request body for deregistering a worker.
type DeregisterWorkerRequest struct {
	Reason        string `json:"reason,omitempty"`
	JobsCompleted int    `json:"jobsCompleted"`
	JobsFailed    int    `json:"jobsFailed"`
}

// InstructionConfig represents the instruction template configuration from API.
// This defines *what* to execute (the template). The CLI's harness layer
// executes instructions.
type InstructionConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Instruction string `json:"instruction"` // Jinja2 template (not rendered)
}

// SandboxConfig represents sandbox configuration for restricted execution.
type SandboxConfig struct {
	Enabled        bool     `json:"enabled"`
	AllowNetwork   bool     `json:"allowNetwork"`
	AllowFileWrite bool     `json:"allowFileWrite"`
	AllowedPaths   []string `json:"allowedPaths"`
}

// HarnessConstraints holds harness-agnostic execution limits.
type HarnessConstraints struct {
	// MaxTurns limits the number of agentic turns (API round-trips) before stopping.
	MaxTurns int `json:"maxTurns,omitempty"`

	// MaxBudgetUSD caps API spending in USD.
	MaxBudgetUSD float64 `json:"maxBudgetUsd,omitempty"`

	// TimeoutMs overrides the job timeout in milliseconds.
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// ClaudeConfig holds Claude-specific execution settings.
type ClaudeConfig struct {
	// AllowedTools restricts which tools Claude can use (e.g., ["Read", "Bash", "Edit"]).
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DisallowedTools blocks specific tools (e.g., ["Task"] to disable subtasks).
	DisallowedTools []string `json:"disallowedTools,omitempty"`

	// SystemPromptAppend is text appended to the system prompt.
	SystemPromptAppend string `json:"systemPromptAppend,omitempty"`
}

// ExecutionConfig contains everything needed to execute a job.
// The server renders the Jinja2 template and provides the fully prepared instruction.
type ExecutionConfig struct {
	// HarnessType specifies which harness to use ("claude", "codex").
	HarnessType string `json:"harnessType,omitempty"`

	// RenderedInstruction is the fully rendered prompt/command (template already applied).
	RenderedInstruction string `json:"renderedInstruction,omitempty"`

	// TimeoutMs is the execution timeout in milliseconds.
	TimeoutMs int `json:"timeoutMs"`

	// WorkingDirectory is the optional working directory for execution.
	WorkingDirectory string `json:"workingDirectory,omitempty"`

	// Environment contains environment variables to set for execution.
	Environment map[string]string `json:"environment,omitempty"`

	// Sandbox contains optional sandbox configuration.
	Sandbox *SandboxConfig `json:"sandbox,omitempty"`

	// Constraints holds harness-agnostic execution limits.
	Constraints *HarnessConstraints `json:"constraints,omitempty"`

	// Claude holds Claude-specific configuration (when HarnessType is "claude").
	Claude *ClaudeConfig `json:"claude,omitempty"`
}

// GetHarnessType returns the harness type.
func (e *ExecutionConfig) GetHarnessType() string {
	return e.HarnessType
}

// GetRenderedInstruction returns the rendered instruction.
func (e *ExecutionConfig) GetRenderedInstruction() string {
	return e.RenderedInstruction
}

// HabitatSummary represents a habitat for CLI selection.
type HabitatSummary struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	HabitatType string `json:"habitatType"`
}

// QueueSummary represents a queue for CLI selection.
type QueueSummary struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	HabitatID string `json:"habitatId,omitempty"`
}

type queueListResponse struct {
	Data []QueueSummary `json:"data"`
}

// InstructionAvailability represents queue instruction availability for runners.
type InstructionAvailability struct {
	QueueID              string `json:"queueId"`
	HasActiveInstruction bool   `json:"hasActiveInstruction"`
	InstructionID        string `json:"instructionId,omitempty"`
	InstructionName      string `json:"instructionName,omitempty"`
	InstructionSlug      string `json:"instructionSlug,omitempty"`
}

// Job represents a job claimed from the queue.
type Job struct {
	ID                  string         `json:"id"`
	OrganizationID      string         `json:"organizationId"`
	JobType             string         `json:"jobType"`
	CeType              string         `json:"ceType"`
	CeSource            string         `json:"ceSource"`
	CeSubject           string         `json:"ceSubject,omitempty"`
	Data                map[string]any `json:"data,omitempty"`
	RouteID             string         `json:"routeId,omitempty"`
	QueueID             string         `json:"queueId,omitempty"`
	HabitatID           string         `json:"habitatId,omitempty"`
	Priority            string         `json:"priority"`
	Status              string         `json:"status"`
	StatusReason        string         `json:"statusReason,omitempty"`
	WorkerID            string         `json:"workerId,omitempty"`
	ClaimedAt           *time.Time     `json:"claimedAt,omitempty"`
	HeartbeatDeadlineAt *time.Time     `json:"heartbeatDeadlineAt,omitempty"`
	AttemptNumber       int            `json:"attemptNumber"`
	MaxAttempts         int            `json:"maxAttempts"`
	NextRetryAt         *time.Time     `json:"nextRetryAt,omitempty"`
	InputData           map[string]any `json:"inputData,omitempty"`
	OutputData          map[string]any `json:"outputData,omitempty"`
	ErrorCode           string         `json:"errorCode,omitempty"`
	ErrorMessage        string         `json:"errorMessage,omitempty"`
	ErrorDetails        map[string]any `json:"errorDetails,omitempty"`
	StartedAt           *time.Time     `json:"startedAt,omitempty"`
	CompletedAt         *time.Time     `json:"completedAt,omitempty"`
	DurationMs          *int           `json:"durationMs,omitempty"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           *time.Time     `json:"updatedAt,omitempty"`

	Instruction    *InstructionConfig `json:"-"`
	Execution      *ExecutionConfig   `json:"-"`
	WebhookConfig  map[string]any     `json:"-"`
	ExecutionError string             `json:"-"`
}

// UnmarshalJSON accepts both organization-scoped and legacy workspace-scoped job payloads.
func (j *Job) UnmarshalJSON(data []byte) error {
	type jobAlias struct {
		ID                  string         `json:"id"`
		OrganizationID      string         `json:"organizationId"`
		WorkspaceID         string         `json:"workspaceId"`
		JobType             string         `json:"jobType"`
		CeType              string         `json:"ceType"`
		CeSource            string         `json:"ceSource"`
		CeSubject           string         `json:"ceSubject,omitempty"`
		Data                map[string]any `json:"data,omitempty"`
		RouteID             string         `json:"routeId,omitempty"`
		QueueID             string         `json:"queueId,omitempty"`
		HabitatID           string         `json:"habitatId,omitempty"`
		Priority            string         `json:"priority"`
		Status              string         `json:"status"`
		StatusReason        string         `json:"statusReason,omitempty"`
		WorkerID            string         `json:"workerId,omitempty"`
		ClaimedAt           *time.Time     `json:"claimedAt,omitempty"`
		HeartbeatDeadlineAt *time.Time     `json:"heartbeatDeadlineAt,omitempty"`
		AttemptNumber       int            `json:"attemptNumber"`
		MaxAttempts         int            `json:"maxAttempts"`
		NextRetryAt         *time.Time     `json:"nextRetryAt,omitempty"`
		InputData           map[string]any `json:"inputData,omitempty"`
		OutputData          map[string]any `json:"outputData,omitempty"`
		ErrorCode           string         `json:"errorCode,omitempty"`
		ErrorMessage        string         `json:"errorMessage,omitempty"`
		ErrorDetails        map[string]any `json:"errorDetails,omitempty"`
		StartedAt           *time.Time     `json:"startedAt,omitempty"`
		CompletedAt         *time.Time     `json:"completedAt,omitempty"`
		DurationMs          *int           `json:"durationMs,omitempty"`
		CreatedAt           time.Time      `json:"createdAt"`
		UpdatedAt           *time.Time     `json:"updatedAt,omitempty"`
	}

	var aux jobAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("unmarshal job: %w", err)
	}

	j.ID = aux.ID
	j.OrganizationID = firstNonEmpty(aux.OrganizationID, aux.WorkspaceID)
	j.JobType = aux.JobType
	j.CeType = aux.CeType
	j.CeSource = aux.CeSource
	j.CeSubject = aux.CeSubject
	j.Data = aux.Data
	j.RouteID = aux.RouteID
	j.QueueID = aux.QueueID
	j.HabitatID = aux.HabitatID
	j.Priority = aux.Priority
	j.Status = aux.Status
	j.StatusReason = aux.StatusReason
	j.WorkerID = aux.WorkerID
	j.ClaimedAt = aux.ClaimedAt
	j.HeartbeatDeadlineAt = aux.HeartbeatDeadlineAt
	j.AttemptNumber = aux.AttemptNumber
	j.MaxAttempts = aux.MaxAttempts
	j.NextRetryAt = aux.NextRetryAt
	j.InputData = aux.InputData
	j.OutputData = aux.OutputData
	j.ErrorCode = aux.ErrorCode
	j.ErrorMessage = aux.ErrorMessage
	j.ErrorDetails = aux.ErrorDetails
	j.StartedAt = aux.StartedAt
	j.CompletedAt = aux.CompletedAt
	j.DurationMs = aux.DurationMs
	j.CreatedAt = aux.CreatedAt
	j.UpdatedAt = aux.UpdatedAt

	return nil
}

// JobClaimResponse wraps the claim response payload.
type JobClaimResponse struct {
	Job            Job                `json:"job"`
	WebhookConfig  map[string]any     `json:"webhookConfig,omitempty"`
	Instruction    *InstructionConfig `json:"instruction,omitempty"`
	Execution      *ExecutionConfig   `json:"execution,omitempty"`
	ExecutionError string             `json:"executionError,omitempty"`
}

// GetHarnessType returns the harness type for this job.
func (j *Job) GetHarnessType() string {
	if j.Execution != nil {
		if harnessType := j.Execution.GetHarnessType(); harnessType != "" {
			return harnessType
		}
	}

	return ""
}

// GetRenderedInstruction returns the rendered instruction for execution.
func (j *Job) GetRenderedInstruction() string {
	if j.Execution != nil {
		return j.Execution.GetRenderedInstruction()
	}

	return ""
}

// GetDisplayName returns a human-friendly job label.
func (j *Job) GetDisplayName() string {
	if j.InputData != nil {
		if name, ok := j.InputData["name"].(string); ok && name != "" {
			return name
		}

		if title, ok := j.InputData["title"].(string); ok && title != "" {
			return title
		}
	}

	return "Job"
}

// New creates a new API client with the given base URL and API key.
func New(baseURL, apiKey string) *Client {
	return NewWithHTTPClient(baseURL, apiKey, nil)
}

// NewWithHTTPClient creates a new API client with an injected HTTP client.
// If httpClient is nil, a default client with DefaultTimeout is used.
func NewWithHTTPClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	if httpClient.Timeout == 0 {
		httpClient.Timeout = DefaultTimeout
	}

	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// ValidateKey validates the API key and returns the runner identity.
func (c *Client) ValidateKey(ctx context.Context) (*Identity, error) {
	identity, _, err := c.ValidateKeyWithMeta(ctx)
	return identity, err
}

// ValidateKeyWithMeta validates the API key and returns identity plus
// request/trace metadata from the response headers.
func (c *Client) ValidateKeyWithMeta(ctx context.Context) (*Identity, *ResponseMeta, error) {
	req, err := c.newRequest(ctx, "GET", c.baseURL+"/v1/runner/me", http.NoBody)
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.do(req, "/v1/runner/me")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to API: %w", err)
	}
	defer resp.Body.Close()

	meta := &ResponseMeta{
		RequestID: strings.TrimSpace(resp.Header.Get("X-Request-Id")),
		TraceID:   responseTraceID(resp),
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, meta, fmt.Errorf("invalid or expired API key")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, meta, fmt.Errorf("API key does not have runner permissions")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, meta, unexpectedStatus("validate key", resp)
	}

	var identity Identity
	if err := decodeJSON(resp.Body, &identity, "failed to parse identity"); err != nil {
		return nil, meta, err
	}

	return &identity, meta, nil
}

// GetCurrentUserProfile fetches the current authenticated user profile.
func (c *Client) GetCurrentUserProfile(ctx context.Context) (*UserProfile, error) {
	req, err := c.newRequest(ctx, "GET", c.baseURL+"/v1/users/me", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, "/v1/users/me")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch current user profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("not authenticated")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("current user profile", resp)
	}

	var profile UserProfile
	if err := decodeJSON(resp.Body, &profile, "failed to parse current user profile"); err != nil {
		return nil, err
	}

	return &profile, nil
}

// GetRunnerConfig fetches runner runtime configuration for startup provisioning.
func (c *Client) GetRunnerConfig(ctx context.Context) (*RunnerConfigResponse, error) {
	req, err := c.newRequest(ctx, "GET", c.baseURL+"/v1/runner/config", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, "/v1/runner/config")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch runner config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("runner config", resp)
	}

	var cfg RunnerConfigResponse
	if err := decodeJSON(resp.Body, &cfg, "failed to parse runner config"); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// IsAuthenticated returns true if the client has an API key configured.
func (c *Client) IsAuthenticated() bool {
	return c.apiKey != ""
}

func (c *Client) setRequestHeaders(req *http.Request) {
	requestID := req.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
		req.Header.Set("X-Request-Id", requestID)
	}

	spanCtx := trace.SpanContextFromContext(req.Context())
	if spanCtx.IsValid() {
		req.Header.Set("X-Trace-Id", spanCtx.TraceID().String())
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mush/"+buildinfo.Version)
}

func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	return req, nil
}

// newPublicRequest creates a request without the Authorization header (for public endpoints).
func (c *Client) newPublicRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mush/"+buildinfo.Version)
	req.Header.Set("X-Request-Id", uuid.NewString())

	spanCtx := trace.SpanContextFromContext(req.Context())
	if spanCtx.IsValid() {
		req.Header.Set("X-Trace-Id", spanCtx.TraceID().String())
	}

	return req, nil
}

func (c *Client) do(req *http.Request, route string) (*http.Response, error) {
	requestID := strings.TrimSpace(req.Header.Get("X-Request-Id"))
	logger := observability.FromContext(req.Context()).With(
		slog.String("component", "client"),
		slog.String("http.request.method", req.Method),
		slog.String("http.route", route),
		slog.String("request.id", requestID),
	)

	start := time.Now()

	logger.Debug("request started", slog.String("event.type", "http.request.start"))

	resp, err := c.httpClient.Do(req)
	durationMS := time.Since(start).Milliseconds()

	if err != nil {
		logger.Error(
			"request failed",
			slog.String("event.type", "http.request.error"),
			slog.Int64("duration_ms", durationMS),
			slog.String("error", err.Error()),
		)

		return nil, &RequestError{
			Operation: "http request",
			RequestID: requestID,
			Cause:     err,
		}
	}

	traceID := responseTraceID(resp)
	if traceID != "" {
		logger = logger.With(slog.String("trace.id", traceID))
	}

	logger.Debug(
		"request completed",
		slog.String("event.type", "http.request.finish"),
		slog.Int("http.response.status_code", resp.StatusCode),
		slog.Int64("duration_ms", durationMS),
		slog.String("trace.id", traceID),
	)

	return resp, nil
}

func encodeJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	return data, nil
}

func decodeJSON(body io.Reader, dst any, msg string) error {
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

func emptyJSONBody() io.Reader {
	return strings.NewReader("{}")
}

// unexpectedStatus creates a formatted error from an unexpected HTTP status code.
func unexpectedStatus(operation string, resp *http.Response) error {
	statusCode := 0
	requestID := ""
	traceID := ""

	if resp != nil {
		statusCode = resp.StatusCode
		requestID = strings.TrimSpace(resp.Header.Get("X-Request-Id"))
		traceID = responseTraceID(resp)
		_, _ = io.Copy(io.Discard, resp.Body)
	}

	return &HTTPStatusError{
		Operation: operation,
		Status:    statusCode,
		RequestID: requestID,
		TraceID:   traceID,
	}
}

func responseTraceID(resp *http.Response) string {
	if resp == nil {
		return ""
	}

	if direct := strings.TrimSpace(resp.Header.Get("X-Trace-Id")); direct != "" {
		return direct
	}

	traceparent := strings.TrimSpace(resp.Header.Get("traceparent"))
	if traceparent == "" {
		return ""
	}

	parts := strings.Split(traceparent, "-")
	if len(parts) < 4 {
		return ""
	}

	return parts[1]
}
