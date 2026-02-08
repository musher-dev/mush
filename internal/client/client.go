// Package client provides the API client for communicating with the Musher platform.
//
// The client handles authentication and provides methods for:
//   - Validating API keys (service accounts)
//   - Claiming jobs from queues
//   - Reporting job completion/failure
//   - Sending job heartbeats
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/musher-dev/mush/internal/buildinfo"
)

const (
	// DefaultBaseURL is the default API endpoint.
	DefaultBaseURL = "http://localhost:17200"
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

// Identity represents the authenticated service account identity.
type Identity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`     // For display purposes
	Workspace string `json:"workspace"` // Workspace name
}

// JobClaimRequest is the request body for claiming a job.
type JobClaimRequest struct {
	QueueID         string `json:"queueId,omitempty"`
	HabitatID       string `json:"habitatId,omitempty"`
	LeaseDurationMs int    `json:"leaseDurationMs"`
}

// JobCompleteRequest is the request body for completing a job.
type JobCompleteRequest struct {
	OutputData map[string]interface{} `json:"outputData,omitempty"`
}

// JobFailRequest is the request body for failing a job.
type JobFailRequest struct {
	ErrorCode    string                 `json:"errorCode,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	ErrorDetails map[string]interface{} `json:"errorDetails,omitempty"`
	ShouldRetry  bool                   `json:"shouldRetry"`
}

// RegisterLinkRequest is the request body for registering a link.
type RegisterLinkRequest struct {
	InstanceID     string                 `json:"instanceId"`
	HabitatID      string                 `json:"habitatId"`
	Name           string                 `json:"name,omitempty"`
	LinkType       string                 `json:"linkType"`
	ClientVersion  string                 `json:"clientVersion,omitempty"`
	ClientMetadata map[string]interface{} `json:"clientMetadata,omitempty"`
}

// RegisterLinkResponse is the response from registering a link.
type RegisterLinkResponse struct {
	LinkID              string    `json:"linkId"`
	WorkerID            string    `json:"workerId"`
	HeartbeatDeadlineAt time.Time `json:"heartbeatDeadlineAt"`
	HeartbeatIntervalMs int       `json:"heartbeatIntervalMs"`
}

// LinkHeartbeatRequest is the request body for link heartbeat.
type LinkHeartbeatRequest struct {
	CurrentJobID string `json:"currentJobId,omitempty"`
}

// LinkHeartbeatResponse is the response from link heartbeat.
type LinkHeartbeatResponse struct {
	Status              string    `json:"status"`
	HeartbeatDeadlineAt time.Time `json:"heartbeatDeadlineAt"`
}

// DeregisterLinkRequest is the request body for deregistering a link.
type DeregisterLinkRequest struct {
	Reason        string `json:"reason,omitempty"`
	JobsCompleted int    `json:"jobsCompleted"`
	JobsFailed    int    `json:"jobsFailed"`
}

// InstructionConfig represents the instruction template configuration from API.
// This defines *what* to execute (the template). The CLI's Agent interface
// (ClaudeAgent, BashAgent) executes instructions.
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

// AgentConstraints holds agent-agnostic execution limits.
type AgentConstraints struct {
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
	// AgentType specifies which agent to use ("claude", "bash").
	AgentType string `json:"agentType,omitempty"`

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

	// Constraints holds agent-agnostic execution limits.
	Constraints *AgentConstraints `json:"constraints,omitempty"`

	// Claude holds Claude-specific configuration (when AgentType is "claude").
	Claude *ClaudeConfig `json:"claude,omitempty"`
}

// GetAgentType returns the agent type.
func (e *ExecutionConfig) GetAgentType() string {
	return e.AgentType
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
	ID                  string                 `json:"id"`
	WorkspaceID         string                 `json:"workspaceId"`
	JobType             string                 `json:"jobType"`
	CeType              string                 `json:"ceType"`
	CeSource            string                 `json:"ceSource"`
	CeSubject           string                 `json:"ceSubject,omitempty"`
	Data                map[string]interface{} `json:"data,omitempty"`
	RouteID             string                 `json:"routeId,omitempty"`
	QueueID             string                 `json:"queueId,omitempty"`
	HabitatID           string                 `json:"habitatId,omitempty"`
	Priority            string                 `json:"priority"`
	Status              string                 `json:"status"`
	StatusReason        string                 `json:"statusReason,omitempty"`
	WorkerID            string                 `json:"workerId,omitempty"`
	ClaimedAt           *time.Time             `json:"claimedAt,omitempty"`
	HeartbeatDeadlineAt *time.Time             `json:"heartbeatDeadlineAt,omitempty"`
	AttemptNumber       int                    `json:"attemptNumber"`
	MaxAttempts         int                    `json:"maxAttempts"`
	NextRetryAt         *time.Time             `json:"nextRetryAt,omitempty"`
	InputData           map[string]interface{} `json:"inputData,omitempty"`
	OutputData          map[string]interface{} `json:"outputData,omitempty"`
	ErrorCode           string                 `json:"errorCode,omitempty"`
	ErrorMessage        string                 `json:"errorMessage,omitempty"`
	ErrorDetails        map[string]interface{} `json:"errorDetails,omitempty"`
	StartedAt           *time.Time             `json:"startedAt,omitempty"`
	CompletedAt         *time.Time             `json:"completedAt,omitempty"`
	DurationMs          *int                   `json:"durationMs,omitempty"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           *time.Time             `json:"updatedAt,omitempty"`

	Instruction    *InstructionConfig     `json:"-"`
	Execution      *ExecutionConfig       `json:"-"`
	WebhookConfig  map[string]interface{} `json:"-"`
	ExecutionError string                 `json:"-"`
}

// JobClaimResponse wraps the claim response payload.
type JobClaimResponse struct {
	Job            Job                    `json:"job"`
	WebhookConfig  map[string]interface{} `json:"webhookConfig,omitempty"`
	Instruction    *InstructionConfig     `json:"instruction,omitempty"`
	Execution      *ExecutionConfig       `json:"execution,omitempty"`
	ExecutionError string                 `json:"executionError,omitempty"`
}

// GetAgentType returns the agent type for this job, checking multiple sources.
func (j *Job) GetAgentType() string {
	// Prefer ExecutionConfig
	if j.Execution != nil {
		if agentType := j.Execution.GetAgentType(); agentType != "" {
			return agentType
		}
	}

	// Default to Claude
	return "claude"
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

// New creates a new API client.
func New(apiKey string) *Client {
	return &Client{
		baseURL: DefaultBaseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// WithBaseURL sets a custom base URL.
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// ValidateKey validates the API key and returns the service account identity.
// Uses the habitats endpoint to validate credentials without requiring a specific habitat.
func (c *Client) ValidateKey(ctx context.Context) (*Identity, error) {
	// Use the runner habitats endpoint to validate credentials
	// This endpoint returns 401 if the key is invalid and doesn't require a habitat ID
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/runner/habitats", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid or expired API key")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("API key does not have runner permissions")
	}

	// Any 2xx means the key is valid
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Extract identity from the API key prefix if possible
		// For now, return a placeholder identity
		return &Identity{
			Email:     "service-account",
			Workspace: "default",
		}, nil
	}

	return nil, unexpectedStatus("validate key", resp.StatusCode, resp.Body)
}

// ClaimJob attempts to claim a job from the queue.
// This is a long-poll endpoint that blocks until a job is available or timeout.
func (c *Client) ClaimJob(ctx context.Context, habitatID, queueID string, waitTimeoutSeconds int) (*Job, error) {
	url := fmt.Sprintf("%s/api/v1/runner/jobs:claim?wait_timeout_seconds=%d", c.baseURL, waitTimeoutSeconds)

	if queueID != "" {
		habitatID = ""
	}
	if queueID == "" && habitatID == "" {
		return nil, fmt.Errorf("must provide either habitatID or queueID")
	}

	body := JobClaimRequest{
		QueueID:         queueID,
		HabitatID:       habitatID,
		LeaseDurationMs: DefaultLeaseDurationMs,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to claim job: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content = no jobs available
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	// 200 with null body = no jobs available
	if resp.StatusCode == http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		trimmed := bytes.TrimSpace(respBody)
		if string(trimmed) == "null" || len(trimmed) == 0 {
			return nil, nil
		}

		var response JobClaimResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("failed to parse job: %w", err)
		}

		job := response.Job
		job.Instruction = response.Instruction
		job.Execution = response.Execution
		job.WebhookConfig = response.WebhookConfig
		job.ExecutionError = response.ExecutionError
		return &job, nil
	}

	return nil, unexpectedStatus("claim job", resp.StatusCode, resp.Body)
}

// StartJob marks a claimed job as running.
func (c *Client) StartJob(ctx context.Context, jobID string) (*Job, error) {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:start", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to start job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("start job", resp.StatusCode, resp.Body)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &job, nil
}

// HeartbeatJob sends a heartbeat for a claimed job to extend the lease.
func (c *Client) HeartbeatJob(ctx context.Context, jobID string) (*Job, error) {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:heartbeat", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("heartbeat job", resp.StatusCode, resp.Body)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &job, nil
}

// CompleteJob marks a job as successfully completed.
func (c *Client) CompleteJob(ctx context.Context, jobID string, output map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:complete", c.baseURL, jobID)

	body := JobCompleteRequest{
		OutputData: output,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("complete job", resp.StatusCode, resp.Body)
	}

	return nil
}

// FailJob marks a job as failed.
func (c *Client) FailJob(ctx context.Context, jobID, errorCode, errorMsg string, shouldRetry bool) error {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:fail", c.baseURL, jobID)

	body := JobFailRequest{
		ErrorCode:    errorCode,
		ErrorMessage: errorMsg,
		ShouldRetry:  shouldRetry,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fail job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("fail job", resp.StatusCode, resp.Body)
	}

	return nil
}

// ReleaseJob releases a job back to the queue without completing.
func (c *Client) ReleaseJob(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:release", c.baseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to release job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("release job", resp.StatusCode, resp.Body)
	}

	return nil
}

func (c *Client) setRequestHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "mush/"+buildinfo.Version)
}

// unexpectedStatus creates a formatted error from an unexpected HTTP status code.
func unexpectedStatus(operation string, statusCode int, body io.Reader) error {
	respBody, readErr := io.ReadAll(body)
	if readErr != nil {
		return fmt.Errorf("%s failed with status %d (failed to read body: %v)", operation, statusCode, readErr)
	}
	return fmt.Errorf("%s failed with status %d: %s", operation, statusCode, string(respBody))
}

// RegisterLink registers a new link with the platform.
// Called on mush start. Returns link ID for subsequent heartbeats/deregister.
func (c *Client) RegisterLink(ctx context.Context, req *RegisterLinkRequest) (*RegisterLinkResponse, error) {
	url := c.baseURL + "/api/v1/runner/links:register"

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to register link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, unexpectedStatus("register link", resp.StatusCode, resp.Body)
	}

	var result RegisterLinkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// HeartbeatLink sends a heartbeat for a link.
// Should be called every 30 seconds to keep the link alive.
func (c *Client) HeartbeatLink(ctx context.Context, linkID, currentJobID string) (*LinkHeartbeatResponse, error) {
	url := fmt.Sprintf("%s/api/v1/runner/links/%s:heartbeat", c.baseURL, linkID)

	req := LinkHeartbeatRequest{
		CurrentJobID: currentJobID,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to heartbeat link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("heartbeat link", resp.StatusCode, resp.Body)
	}

	var result LinkHeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// DeregisterLink gracefully disconnects a link.
// Called on mush shutdown (SIGTERM/SIGINT).
func (c *Client) DeregisterLink(ctx context.Context, linkID string, req DeregisterLinkRequest) error {
	url := fmt.Sprintf("%s/api/v1/runner/links/%s:deregister", c.baseURL, linkID)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to deregister link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("deregister link", resp.StatusCode, resp.Body)
	}

	return nil
}

// ListHabitats fetches available habitats in the runner's workspace.
func (c *Client) ListHabitats(ctx context.Context) ([]HabitatSummary, error) {
	url := c.baseURL + "/api/v1/runner/habitats"

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch habitats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("list habitats", resp.StatusCode, resp.Body)
	}

	var habitats []HabitatSummary
	if err := json.NewDecoder(resp.Body).Decode(&habitats); err != nil {
		return nil, fmt.Errorf("failed to parse habitats: %w", err)
	}

	return habitats, nil
}

// ListQueues fetches active queues, optionally filtered by habitat.
func (c *Client) ListQueues(ctx context.Context, habitatID string) ([]QueueSummary, error) {
	endpoint, err := neturl.Parse(c.baseURL + "/api/v1/queues")
	if err != nil {
		return nil, fmt.Errorf("failed to parse queue endpoint: %w", err)
	}

	query := endpoint.Query()
	query.Set("status", "active")
	query.Set("limit", "100")
	if habitatID != "" {
		query.Set("habitat_id", habitatID)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch queues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("list queues", resp.StatusCode, resp.Body)
	}

	var response queueListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse queues: %w", err)
	}

	return response.Data, nil
}

// GetQueueInstructionAvailability checks if a queue has an active instruction.
func (c *Client) GetQueueInstructionAvailability(ctx context.Context, queueID string) (*InstructionAvailability, error) {
	url := fmt.Sprintf("%s/api/v1/runner/queues/%s/instruction-availability", c.baseURL, queueID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch instruction availability: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("queue not found: %s", queueID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("instruction availability", resp.StatusCode, resp.Body)
	}

	var availability InstructionAvailability
	if err := json.NewDecoder(resp.Body).Decode(&availability); err != nil {
		return nil, fmt.Errorf("failed to parse instruction availability: %w", err)
	}

	return &availability, nil
}
