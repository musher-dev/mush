package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
)

// BundleResolveResponse is the response from resolving a bundle version.
type BundleResolveResponse struct {
	BundleID  string
	VersionID string
	Version   string
	State     string
	OCIRef    string
	OCIDigest string
	Manifest  BundleManifest
}

// BundleManifest describes the layers (assets) in a bundle version.
type BundleManifest struct {
	Layers []BundleLayer `json:"layers"`
}

// BundleLayer describes a single asset in a bundle.
type BundleLayer struct {
	AssetID       string
	LogicalPath   string
	AssetType     string // "skill", "agent_definition", "tool_config"
	ContentSHA256 string
	SizeBytes     int64
}

// UnmarshalJSON supports both camelCase and snake_case payloads.
func (r *BundleResolveResponse) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode bundle resolve response: %w", err)
	}

	r.BundleID = stringOr(raw, "bundleId", "bundle_id")
	r.VersionID = stringOr(raw, "versionId", "version_id")
	r.Version = stringOr(raw, "version")
	r.State = stringOr(raw, "state")
	r.OCIRef = stringOr(raw, "ociRef", "oci_ref")
	r.OCIDigest = stringOr(raw, "ociDigest", "oci_digest")

	if manifestRaw, ok := firstRaw(raw, "manifest"); ok && len(manifestRaw) > 0 {
		if err := json.Unmarshal(manifestRaw, &r.Manifest); err != nil {
			return fmt.Errorf("decode manifest: %w", err)
		}
	}

	return nil
}

// UnmarshalJSON supports both camelCase and snake_case payloads.
func (l *BundleLayer) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode bundle layer: %w", err)
	}

	l.AssetID = stringOr(raw, "assetId", "asset_id")
	l.LogicalPath = stringOr(raw, "logicalPath", "logical_path")
	l.AssetType = stringOr(raw, "assetType", "asset_type")
	l.ContentSHA256 = stringOr(raw, "contentSha256", "content_sha256")
	l.SizeBytes = int64Or(raw, "sizeBytes", "size_bytes")

	return nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve bundle (%s): %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("resolve bundle (%s): status %d", path, resp.StatusCode)
	}

	if resp.StatusCode == http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)

		bodyStr := strings.ToLower(string(bodyBytes))
		if strings.Contains(bodyStr, "version") {
			return nil, fmt.Errorf("resolve bundle (%s): version is required by server", path)
		}

		return nil, unexpectedStatus("resolve bundle", resp.StatusCode, strings.NewReader(string(bodyBytes)))
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
	path := fmt.Sprintf("/api/v1/assets/%s", neturl.PathEscape(assetID))
	return c.fetchBundleAssetAttempt(ctx, path)
}

func (c *Client) fetchBundleAssetAttempt(ctx context.Context, path string) ([]byte, error) {
	endpointURL := c.baseURL + path

	req, err := c.newRequest(ctx, "GET", endpointURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
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

	if content, ok := payload["content_text"].(string); ok {
		return content, true
	}

	return "", false
}

func stringOr(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		valueRaw, ok := raw[key]
		if !ok {
			continue
		}

		var value string
		if err := json.Unmarshal(valueRaw, &value); err == nil {
			return value
		}
	}

	return ""
}

func int64Or(raw map[string]json.RawMessage, keys ...string) int64 {
	for _, key := range keys {
		valueRaw, ok := raw[key]
		if !ok {
			continue
		}

		var value int64
		if err := json.Unmarshal(valueRaw, &value); err == nil {
			return value
		}
	}

	return 0
}

func firstRaw(raw map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, key := range keys {
		valueRaw, ok := raw[key]
		if ok {
			return valueRaw, true
		}
	}

	return nil, false
}
