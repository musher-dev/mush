package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ClaimJob claims a job from a habitat or queue.
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

	jsonBody, err := encodeJSON(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to claim job: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content = no jobs available.
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil //nolint:nilnil // no job available is represented as nil job + nil error
	}

	// 200 with null body = no jobs available.
	if resp.StatusCode == http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		trimmed := bytes.TrimSpace(respBody)
		if string(trimmed) == "null" || len(trimmed) == 0 {
			return nil, nil //nolint:nilnil // no job available is represented as nil job + nil error
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
	return c.updateJobStatus(ctx, jobID, "start", "start job")
}

// HeartbeatJob sends a heartbeat for a claimed job to extend the lease.
func (c *Client) HeartbeatJob(ctx context.Context, jobID string) (*Job, error) {
	return c.updateJobStatus(ctx, jobID, "heartbeat", "heartbeat job")
}

// CompleteJob marks a job as successfully completed.
func (c *Client) CompleteJob(ctx context.Context, jobID string, output map[string]any) error {
	url := fmt.Sprintf("%s/api/v1/runner/jobs/%s:complete", c.baseURL, jobID)

	body := JobCompleteRequest{
		OutputData: output,
	}

	jsonBody, err := encodeJSON(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

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

	jsonBody, err := encodeJSON(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

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

	req, err := c.newRequest(ctx, "POST", url, emptyJSONBody())
	if err != nil {
		return err
	}

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

func (c *Client) updateJobStatus(ctx context.Context, jobID, endpointAction, operation string) (*Job, error) {
	endpointURL := fmt.Sprintf("%s/api/v1/runner/jobs/%s:%s", c.baseURL, jobID, endpointAction)

	req, err := c.newRequest(ctx, "POST", endpointURL, emptyJSONBody())
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to %s: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus(operation, resp.StatusCode, resp.Body)
	}

	var job Job
	if err := decodeJSON(resp.Body, &job, "failed to parse response"); err != nil {
		return nil, err
	}

	return &job, nil
}
