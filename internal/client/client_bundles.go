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

// ResolveBundle resolves a bundle slug (and optional version) to a concrete
// version with manifest by querying hub endpoints.
func (c *Client) ResolveBundle(ctx context.Context, namespace, slug, version string) (*BundleResolveResponse, error) {
	// 1. Get bundle detail from hub (public, no auth required).
	detail, err := c.GetHubBundleDetail(ctx, namespace, slug)
	if err != nil {
		return nil, fmt.Errorf("resolve bundle: %w", err)
	}

	resolvedVersion := version
	if resolvedVersion == "" {
		resolvedVersion = detail.LatestVersion
	}

	if resolvedVersion == "" {
		return nil, fmt.Errorf("resolve bundle: no published version available for %s/%s", namespace, slug)
	}

	// 2. Get assets list from hub (public, no auth required).
	assetsPath := fmt.Sprintf("/v1/hub/bundles/%s/%s/assets?version=%s",
		neturl.PathEscape(namespace),
		neturl.PathEscape(slug),
		neturl.QueryEscape(resolvedVersion),
	)

	req, err := c.newPublicRequest(ctx, "GET", c.baseURL+assetsPath, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, assetsPath)
	if err != nil {
		return nil, fmt.Errorf("resolve bundle assets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("resolve bundle assets", resp)
	}

	var assetsResp struct {
		Data []struct {
			ID            string `json:"id"`
			AssetType     string `json:"assetType"`
			LogicalPath   string `json:"logicalPath"`
			ContentSHA256 string `json:"contentSha256"`
			SizeBytes     int64  `json:"sizeBytes"`
		} `json:"data"`
	}

	if err := decodeJSON(resp.Body, &assetsResp, "failed to parse bundle assets"); err != nil {
		return nil, err
	}

	// 3. Build resolve response from hub data.
	layers := make([]BundleLayer, len(assetsResp.Data))
	for i, asset := range assetsResp.Data {
		layers[i] = BundleLayer{
			AssetID:       asset.ID,
			LogicalPath:   asset.LogicalPath,
			AssetType:     asset.AssetType,
			ContentSHA256: asset.ContentSHA256,
			SizeBytes:     asset.SizeBytes,
		}
	}

	return &BundleResolveResponse{
		BundleID:  detail.ID,
		Version:   resolvedVersion,
		Namespace: namespace,
		Slug:      slug,
		Ref:       namespace + "/" + slug,
		State:     "published",
		Manifest:  BundleManifest{Layers: layers},
	}, nil
}

// PullBundleResponse is the response from the single-request pull endpoint.
// It contains all asset content inline, eliminating the need for per-asset fetches.
type PullBundleResponse struct {
	Namespace   string            `json:"namespace"`
	Slug        string            `json:"slug"`
	Version     string            `json:"version"`
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	Manifest    []PullBundleAsset `json:"manifest"`
}

// PullBundleAsset is a single asset with inline content from the pull endpoint.
type PullBundleAsset struct {
	LogicalPath string  `json:"logicalPath"`
	AssetType   string  `json:"assetType"`
	ContentText string  `json:"contentText"`
	MediaType   *string `json:"mediaType"`
}

// PullBundle fetches a bundle version with all asset content in a single request.
// This is the preferred download method — it returns everything inline and handles
// OCI-to-DB fallback server-side. Public endpoint, no auth required.
func (c *Client) PullBundle(ctx context.Context, namespace, slug, version string) (*PullBundleResponse, error) {
	path := fmt.Sprintf("/v1/hub/bundles/%s/%s/versions/%s:pull",
		neturl.PathEscape(namespace),
		neturl.PathEscape(slug),
		neturl.PathEscape(version),
	)

	req, err := c.newPublicRequest(ctx, "GET", c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req, path)
	if err != nil {
		return nil, fmt.Errorf("pull bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("pull bundle", resp)
	}

	var result PullBundleResponse
	if err := decodeJSON(resp.Body, &result, "failed to parse pull response"); err != nil {
		return nil, err
	}

	return &result, nil
}

// ErrNullContent indicates the server returned a null contentText for an asset.
// This typically means the server could not resolve the asset content (e.g. OCI
// registry unavailable for a registry-sourced bundle).
var ErrNullContent = errors.New("asset content unavailable from server")

// FetchBundleAsset downloads a single asset by ID and returns its raw content.
// The asset ID is already version-specific, so no version query param is needed.
func (c *Client) FetchBundleAsset(ctx context.Context, assetID string) ([]byte, error) {
	path := fmt.Sprintf("/v1/runner/assets/%s", neturl.PathEscape(assetID))
	return c.fetchAsset(ctx, path, true)
}

// FetchHubBundleAsset downloads a single asset via the public hub asset-by-path
// endpoint. This serves as a fallback when the runner asset endpoint fails
// (e.g. OCI registry unavailable for registry-sourced bundles).
func (c *Client) FetchHubBundleAsset(ctx context.Context, namespace, slug, logicalPath, version string) ([]byte, error) {
	path := fmt.Sprintf("/v1/hub/bundles/%s/%s/assets/%s?version=%s",
		neturl.PathEscape(namespace),
		neturl.PathEscape(slug),
		logicalPath,
		neturl.QueryEscape(version),
	)

	return c.fetchAsset(ctx, path, false)
}

func (c *Client) fetchAsset(ctx context.Context, path string, authenticated bool) ([]byte, error) {
	endpointURL := c.baseURL + path

	var (
		req *http.Request
		err error
	)

	if authenticated {
		req, err = c.newRequest(ctx, "GET", endpointURL, http.NoBody)
	} else {
		req, err = c.newPublicRequest(ctx, "GET", endpointURL, http.NoBody)
	}

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
