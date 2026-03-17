package client

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type bundleRoundTripFunc func(*http.Request) (*http.Response, error)

func (f bundleRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func bundleJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func bundleRawResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// bundleResolveMock returns a transport that serves hub detail and assets endpoints
// for ResolveBundle tests.
func bundleResolveMock(t *testing.T, detailJSON, assetsJSON string) *http.Client {
	t.Helper()

	return &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "/assets"):
				return bundleJSONResponse(http.StatusOK, assetsJSON), nil
			case strings.HasPrefix(r.URL.Path, "/v1/hub/bundles/"):
				return bundleJSONResponse(http.StatusOK, detailJSON), nil
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
				return nil, nil
			}
		}),
	}
}

func TestAnonymousClientSkipsAuthHeader(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("Authorization header = %q, want empty for anonymous client", got)
			}

			switch {
			case strings.Contains(r.URL.Path, "/assets"):
				return bundleJSONResponse(http.StatusOK, `{"data":[{"id":"a1","assetType":"skill","logicalPath":"skill.md","contentSha256":"abc","sizeBytes":10}]}`), nil
			default:
				return bundleJSONResponse(http.StatusOK, `{"id":"b1","slug":"public-bundle","latestVersion":"1.0.0","publisher":{"handle":"pub"}}`), nil
			}
		}),
	}

	c := NewWithHTTPClient("https://example.test", "", clientHTTP)

	if c.IsAuthenticated() {
		t.Fatal("IsAuthenticated() = true, want false for anonymous client")
	}

	resolved, err := c.ResolveBundle(t.Context(), "pub", "public-bundle", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.BundleID != "b1" {
		t.Fatalf("ResolveBundle() BundleID = %q, want b1", resolved.BundleID)
	}
}

func TestAuthenticatedClientSendsAuthHeader(t *testing.T) {
	t.Parallel()

	// ResolveBundle uses public (no-auth) hub endpoints, so auth header should NOT be sent.
	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "/assets"):
				return bundleJSONResponse(http.StatusOK, `{"data":[{"id":"a1","assetType":"skill","logicalPath":"skill.md","contentSha256":"abc","sizeBytes":10}]}`), nil
			default:
				return bundleJSONResponse(http.StatusOK, `{"id":"b2","slug":"private-bundle","latestVersion":"2.0.0","publisher":{"handle":"priv"}}`), nil
			}
		}),
	}

	c := NewWithHTTPClient("https://example.test", "my-key", clientHTTP)

	if !c.IsAuthenticated() {
		t.Fatal("IsAuthenticated() = false, want true for authenticated client")
	}

	resolved, err := c.ResolveBundle(t.Context(), "priv", "private-bundle", "2.0.0")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.BundleID != "b2" {
		t.Fatalf("ResolveBundle() BundleID = %q, want b2", resolved.BundleID)
	}
}

func TestResolveBundleUsesHubEndpoints(t *testing.T) {
	t.Parallel()

	clientHTTP := bundleResolveMock(t,
		`{"id":"b1","slug":"my-bundle","latestVersion":"1.2.3","publisher":{"handle":"acme"}}`,
		`{"data":[{"id":"a1","assetType":"skill","logicalPath":"skill.md","contentSha256":"abc123","sizeBytes":100}]}`,
	)

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	resolved, err := c.ResolveBundle(t.Context(), "acme", "my-bundle", "1.2.3")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.BundleID != "b1" {
		t.Fatalf("ResolveBundle() BundleID = %q, want b1", resolved.BundleID)
	}

	if len(resolved.Manifest.Layers) != 1 || resolved.Manifest.Layers[0].AssetID != "a1" {
		t.Fatalf("ResolveBundle() manifest layers = %#v, want 1 layer with assetId=a1", resolved.Manifest.Layers)
	}
}

func TestResolveBundleWithoutVersionUsesLatest(t *testing.T) {
	t.Parallel()

	clientHTTP := bundleResolveMock(t,
		`{"id":"b3","slug":"my-bundle","latestVersion":"9.9.9","publisher":{"handle":"acme"}}`,
		`{"data":[{"id":"a1","assetType":"skill","logicalPath":"skill.md","contentSha256":"def456","sizeBytes":50}]}`,
	)

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	resolved, err := c.ResolveBundle(t.Context(), "acme", "my-bundle", "")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.Version != "9.9.9" {
		t.Fatalf("ResolveBundle() version = %q, want 9.9.9", resolved.Version)
	}
}

func TestResolveBundleReturnsErrorOnNotFound(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return bundleJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	_, err := c.ResolveBundle(t.Context(), "acme", "my-bundle", "")
	if err == nil {
		t.Fatal("ResolveBundle() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ResolveBundle() error = %v, want not found", err)
	}
}

func TestFetchBundleAssetParsesJSONContentText(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/runner/assets/asset-1" {
				t.Fatalf("path = %q, want /v1/runner/assets/asset-1", r.URL.Path)
			}

			return bundleJSONResponse(http.StatusOK, `{"id":"asset-1","contentText":"hello world"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	data, err := c.FetchBundleAsset(t.Context(), "asset-1", "")
	if err != nil {
		t.Fatalf("FetchBundleAsset() error = %v", err)
	}

	if string(data) != "hello world" {
		t.Fatalf("FetchBundleAsset() data = %q, want %q", string(data), "hello world")
	}
}

func TestFetchBundleAssetSupportsRawAssetPayload(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/runner/assets/asset-2" {
				t.Fatalf("path = %q, want /v1/runner/assets/asset-2", r.URL.Path)
			}

			return bundleRawResponse(http.StatusOK, "raw content"), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	data, err := c.FetchBundleAsset(t.Context(), "asset-2", "")
	if err != nil {
		t.Fatalf("FetchBundleAsset() error = %v", err)
	}

	if string(data) != "raw content" {
		t.Fatalf("FetchBundleAsset() data = %q, want %q", string(data), "raw content")
	}
}

func TestFetchBundleAssetSendsVersionQueryParam(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.URL.Query().Get("version"); got != "2.0.0" {
				t.Fatalf("version query param = %q, want 2.0.0", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"id":"asset-3","contentText":"versioned"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	data, err := c.FetchBundleAsset(t.Context(), "asset-3", "2.0.0")
	if err != nil {
		t.Fatalf("FetchBundleAsset() error = %v", err)
	}

	if string(data) != "versioned" {
		t.Fatalf("FetchBundleAsset() data = %q, want %q", string(data), "versioned")
	}
}

func TestFetchBundleAssetNullContentTextReturnsError(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return bundleJSONResponse(http.StatusOK, `{"id":"asset-4","contentText":null}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	_, err := c.FetchBundleAsset(t.Context(), "asset-4", "")
	if err == nil {
		t.Fatal("FetchBundleAsset() expected error for null contentText, got nil")
	}

	if !errors.Is(err, ErrNullContent) {
		t.Fatalf("FetchBundleAsset() error = %v, want ErrNullContent", err)
	}
}
