package client

import (
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

func TestResolveBundleUsesBundlesResolveEndpoint(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/bundles:resolve" {
				t.Fatalf("path = %q, want /api/v1/bundles:resolve", r.URL.Path)
			}

			if got := r.URL.Query().Get("bundle_slug"); got != "my-bundle" {
				t.Fatalf("bundle_slug = %q, want my-bundle", got)
			}

			if got := r.URL.Query().Get("version"); got != "1.2.3" {
				t.Fatalf("version = %q, want 1.2.3", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"bundleId":"b1","versionId":"v1","version":"1.2.3","state":"published","ociRef":"registry.example/ws/my-bundle:1.2.3","ociDigest":"sha256:abc"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	resolved, err := c.ResolveBundle(t.Context(), "my-bundle", "1.2.3")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.BundleID != "b1" || resolved.VersionID != "v1" {
		t.Fatalf("ResolveBundle() IDs = (%q,%q), want (b1,v1)", resolved.BundleID, resolved.VersionID)
	}

	if resolved.OCIRef == "" || resolved.OCIDigest == "" {
		t.Fatalf("ResolveBundle() missing OCI fields: %#v", resolved)
	}
}

func TestResolveBundleWithoutVersionUsesLatest(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/bundles:resolve" {
				t.Fatalf("path = %q, want /api/v1/bundles:resolve", r.URL.Path)
			}

			if got := r.URL.Query().Get("bundle_slug"); got != "my-bundle" {
				t.Fatalf("bundle_slug = %q, want my-bundle", got)
			}

			if got := r.URL.Query().Get("version"); got != "" {
				t.Fatalf("version = %q, want empty", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"bundleId":"b3","versionId":"v3","version":"9.9.9","state":"published","ociRef":"registry.example/ws/my-bundle:9.9.9","ociDigest":"sha256:def"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	resolved, err := c.ResolveBundle(t.Context(), "my-bundle", "")
	if err != nil {
		t.Fatalf("ResolveBundle() error = %v", err)
	}

	if resolved.Version != "9.9.9" {
		t.Fatalf("ResolveBundle() version = %q, want 9.9.9", resolved.Version)
	}
}

func TestResolveBundleReturnsBadRequestError(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return bundleJSONResponse(http.StatusBadRequest, `{"detail":"invalid request"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	_, err := c.ResolveBundle(t.Context(), "my-bundle", "")
	if err == nil {
		t.Fatal("ResolveBundle() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("ResolveBundle() error = %v, want status 400", err)
	}
}

func TestFetchBundleAssetParsesJSONContentText(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/v1/runner/assets/asset-1" {
				t.Fatalf("path = %q, want /api/v1/runner/assets/asset-1", r.URL.Path)
			}

			return bundleJSONResponse(http.StatusOK, `{"id":"asset-1","contentText":"hello world"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	data, err := c.FetchBundleAsset(t.Context(), "asset-1")
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
			if r.URL.Path != "/api/v1/runner/assets/asset-2" {
				t.Fatalf("path = %q, want /api/v1/runner/assets/asset-2", r.URL.Path)
			}

			return bundleRawResponse(http.StatusOK, "raw content"), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "test-key", clientHTTP)

	data, err := c.FetchBundleAsset(t.Context(), "asset-2")
	if err != nil {
		t.Fatalf("FetchBundleAsset() error = %v", err)
	}

	if string(data) != "raw content" {
		t.Fatalf("FetchBundleAsset() data = %q, want %q", string(data), "raw content")
	}
}
