package client

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
)

// ListHabitats lists habitats available to the authenticated service account.
func (c *Client) ListHabitats(ctx context.Context) ([]HabitatSummary, error) {
	url := c.baseURL + "/api/v1/runner/habitats"

	req, err := c.newRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list habitats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("list habitats", resp.StatusCode, resp.Body)
	}

	var habitats []HabitatSummary
	if err := decodeJSON(resp.Body, &habitats, "failed to parse habitats response"); err != nil {
		return nil, err
	}

	return habitats, nil
}

// ListQueues lists queues for a habitat.
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

	req, err := c.newRequest(ctx, "GET", endpoint.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list queues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("list queues", resp.StatusCode, resp.Body)
	}

	var response queueListResponse
	if err := decodeJSON(resp.Body, &response, "failed to parse queues"); err != nil {
		return nil, err
	}

	return response.Data, nil
}

// GetQueueInstructionAvailability returns whether a queue has active instructions.
func (c *Client) GetQueueInstructionAvailability(ctx context.Context, queueID string) (*InstructionAvailability, error) {
	if queueID == "" {
		return nil, fmt.Errorf("queueID is required")
	}

	endpointURL := fmt.Sprintf("%s/api/v1/runner/queues/%s/instruction-availability", c.baseURL, neturl.PathEscape(queueID))

	req, err := c.newRequest(ctx, "GET", endpointURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instruction availability: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("get instruction availability", resp.StatusCode, resp.Body)
	}

	var availability InstructionAvailability
	if err := decodeJSON(resp.Body, &availability, "failed to parse instruction availability response"); err != nil {
		return nil, err
	}

	return &availability, nil
}
