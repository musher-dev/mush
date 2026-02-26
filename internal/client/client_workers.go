package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// RegisterWorker registers a new worker with the platform.
// Called on mush start. Returns worker ID for subsequent heartbeats/deregister.
func (c *Client) RegisterWorker(ctx context.Context, req *RegisterWorkerRequest) (*RegisterWorkerResponse, error) {
	url := c.baseURL + "/api/v1/runner/workers:register"

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/workers:register")
	if err != nil {
		return nil, fmt.Errorf("failed to register worker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, unexpectedStatus("register worker", resp.StatusCode, resp.Body)
	}

	var result RegisterWorkerResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// HeartbeatWorker sends a heartbeat for a worker.
// Should be called every 30 seconds to keep the worker alive.
func (c *Client) HeartbeatWorker(ctx context.Context, workerID, currentJobID string) (*WorkerHeartbeatResponse, error) {
	url := fmt.Sprintf("%s/api/v1/runner/workers/%s:heartbeat", c.baseURL, workerID)

	req := WorkerHeartbeatRequest{
		CurrentJobID: currentJobID,
	}

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/workers/{worker_id}:heartbeat")
	if err != nil {
		return nil, fmt.Errorf("failed to heartbeat worker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("heartbeat worker", resp.StatusCode, resp.Body)
	}

	var result WorkerHeartbeatResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeregisterWorker gracefully disconnects a worker.
// Called on mush shutdown (SIGTERM/SIGINT).
func (c *Client) DeregisterWorker(ctx context.Context, workerID string, req DeregisterWorkerRequest) error {
	url := fmt.Sprintf("%s/api/v1/runner/workers/%s:deregister", c.baseURL, workerID)

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/workers/{worker_id}:deregister")
	if err != nil {
		return fmt.Errorf("failed to deregister worker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("deregister worker", resp.StatusCode, resp.Body)
	}

	return nil
}
