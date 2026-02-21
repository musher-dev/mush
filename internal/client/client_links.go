package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// RegisterLink registers a new link with the platform.
// Called on mush start. Returns link ID for subsequent heartbeats/deregister.
func (c *Client) RegisterLink(ctx context.Context, req *RegisterLinkRequest) (*RegisterLinkResponse, error) {
	url := c.baseURL + "/api/v1/runner/links:register"

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/links:register")
	if err != nil {
		return nil, fmt.Errorf("failed to register link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, unexpectedStatus("register link", resp.StatusCode, resp.Body)
	}

	var result RegisterLinkResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse response"); err != nil {
		return nil, err
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

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/links/{link_id}:heartbeat")
	if err != nil {
		return nil, fmt.Errorf("failed to heartbeat link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("heartbeat link", resp.StatusCode, resp.Body)
	}

	var result LinkHeartbeatResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeregisterLink gracefully disconnects a link.
// Called on mush shutdown (SIGTERM/SIGINT).
func (c *Client) DeregisterLink(ctx context.Context, linkID string, req DeregisterLinkRequest) error {
	url := fmt.Sprintf("%s/api/v1/runner/links/%s:deregister", c.baseURL, linkID)

	jsonBody, err := encodeJSON(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := c.newRequest(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}

	resp, err := c.do(httpReq, "/api/v1/runner/links/{link_id}:deregister")
	if err != nil {
		return fmt.Errorf("failed to deregister link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus("deregister link", resp.StatusCode, resp.Body)
	}

	return nil
}
