package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
)

// BundleResolveResponse is the response from resolving a bundle version.
type BundleResolveResponse struct {
	BundleID   string         `json:"bundleId"`
	VersionID  string         `json:"versionId"`
	Version    string         `json:"version"`
	Namespace  string         `json:"namespace"`
	Slug       string         `json:"slug"`
	Ref        string         `json:"ref"`
	State      string         `json:"state"`
	SourceType string         `json:"sourceType"`
	OCIRef     string         `json:"ociRef"`
	OCIDigest  string         `json:"ociDigest"`
	Manifest   BundleManifest `json:"manifest"`
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
	MediaType     string `json:"mediaType,omitempty"`
	ContentSHA256 string `json:"contentSha256"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// ResolveBundle resolves a bundle slug (and optional version) to a concrete version with manifest.
func (c *Client) ResolveBundle(ctx context.Context, namespace, slug, version string) (*BundleResolveResponse, error) {
	params := map[string]string{
		"namespace":   namespace,
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
		return nil, unexpectedStatus("resolve bundle", resp)
	}

	var result BundleResolveResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse bundle resolve response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// ErrNullContent indicates the server returned a null contentText for an asset.
// This typically means the server could not resolve the asset content (e.g. OCI
// registry unavailable for a registry-sourced bundle).
var ErrNullContent = errors.New("asset content unavailable from server")

// FetchBundleAsset downloads a single asset by ID and returns its raw content.
// When version is non-empty, it is sent as a query parameter to enable
// server-side caching (Cache-Control: immutable).
func (c *Client) FetchBundleAsset(ctx context.Context, assetID, version string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/runner/assets/%s", neturl.PathEscape(assetID))
	if version != "" {
		path += "?version=" + neturl.QueryEscape(version)
	}

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
		return nil, unexpectedStatus("fetch bundle asset", resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bundle asset (%s): %w", path, err)
	}

	content, ok, extractErr := extractAssetContent(data)
	if extractErr != nil {
		return nil, fmt.Errorf("fetch bundle asset (%s): %w", path, extractErr)
	}

	if ok {
		return []byte(content), nil
	}

	return data, nil
}

func extractAssetContent(data []byte) (content string, found bool, err error) {
	var payload map[string]any

	if jsonErr := json.Unmarshal(data, &payload); jsonErr != nil {
		return "", false, nil //nolint:nilerr // not JSON — caller falls back to raw bytes
	}

	val, exists := payload["contentText"]
	if !exists {
		return "", false, nil
	}

	if val == nil {
		return "", false, ErrNullContent
	}

	if s, ok := val.(string); ok {
		return s, true, nil
	}

	return "", false, nil
}
