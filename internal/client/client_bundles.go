package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
)

// BundleResolveResponse is the response from resolving a bundle version.
type BundleResolveResponse struct {
	BundleID  string         `json:"bundleId"`
	VersionID string         `json:"versionId"`
	Version   string         `json:"version"`
	State     string         `json:"state"`
	OCIRef    string         `json:"ociRef"`
	OCIDigest string         `json:"ociDigest"`
	Manifest  BundleManifest `json:"manifest"`
}

// BundleManifest describes the layers (assets) in a bundle version.
type BundleManifest struct {
	Layers []BundleLayer `json:"layers"`
}

// BundleLayer describes a single asset in a bundle.
type BundleLayer struct {
	AssetID       string `json:"assetId"`
	LogicalPath   string `json:"logicalPath"`
	AssetType     string `json:"assetType"` // "skill", "agent_definition", "tool_config"
	ContentSHA256 string `json:"contentSha256"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// ResolveBundle resolves a bundle slug (and optional version) to a concrete version with manifest.
func (c *Client) ResolveBundle(ctx context.Context, slug, version string) (*BundleResolveResponse, error) {
	params := map[string]string{
		"bundle_slug": slug,
	}

	if version != "" {
		params["version"] = version
	}

	return c.resolveBundleAttempt(ctx, "/api/v1/bundles:resolve", params)
}

func (c *Client) resolveBundleAttempt(
	ctx context.Context,
	path string,
	params map[string]string,
) (*BundleResolveResponse, error) {
	endpoint, err := neturl.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint %s: %w", path, err)
	}

	query := endpoint.Query()
	for key, value := range params {
		query.Set(key, value)
	}

	endpoint.RawQuery = query.Encode()

	req, err := c.newRequest(ctx, "GET", endpoint.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, path)
	if err != nil {
		return nil, fmt.Errorf("resolve bundle (%s): %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("resolve bundle (%s): status %d", path, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("resolve bundle", resp.StatusCode, resp.Body)
	}

	var result BundleResolveResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse bundle resolve response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// FetchBundleAsset downloads a single asset by ID and returns its raw content.
func (c *Client) FetchBundleAsset(ctx context.Context, assetID string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/runner/assets/%s", neturl.PathEscape(assetID))
	return c.fetchBundleAssetAttempt(ctx, path)
}

func (c *Client) fetchBundleAssetAttempt(ctx context.Context, path string) ([]byte, error) {
	endpointURL := c.baseURL + path

	req, err := c.newRequest(ctx, "GET", endpointURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, path)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle asset (%s): %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("fetch bundle asset (%s): status %d", path, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("fetch bundle asset", resp.StatusCode, resp.Body)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bundle asset (%s): %w", path, err)
	}

	if content, ok := extractAssetContent(data); ok {
		return []byte(content), nil
	}

	return data, nil
}

func extractAssetContent(data []byte) (string, bool) {
	var payload map[string]any

	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false
	}

	if content, ok := payload["contentText"].(string); ok {
		return content, true
	}

	return "", false
}
