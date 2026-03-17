package client

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearchHubBundlesHappyPath(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/hub/bundles" {
				t.Fatalf("path = %q, want /v1/hub/bundles", r.URL.Path)
			}

			if got := r.URL.Query().Get("q"); got != "deploy" {
				t.Fatalf("q = %q, want deploy", got)
			}

			if got := r.URL.Query().Get("sort"); got != "trending" {
				t.Fatalf("sort = %q, want trending", got)
			}

			return bundleJSONResponse(http.StatusOK, `{
				"data": [{"id":"b1","slug":"deploy-tool","displayName":"Deploy Tool","bundleType":"tool","latestVersion":"1.0.0","publisher":{"handle":"acme"}}],
				"meta": {"nextCursor":"cur1","hasMore":true}
			}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "my-key", clientHTTP)

	resp, err := c.SearchHubBundles(t.Context(), "deploy", "", "trending", 20, "")
	if err != nil {
		t.Fatalf("SearchHubBundles() error = %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("SearchHubBundles() data len = %d, want 1", len(resp.Data))
	}

	if resp.Data[0].Slug != "deploy-tool" {
		t.Fatalf("SearchHubBundles() slug = %q, want deploy-tool", resp.Data[0].Slug)
	}

	if !resp.Meta.HasMore {
		t.Fatal("SearchHubBundles() hasMore = false, want true")
	}

	if resp.Meta.NextCursor != "cur1" {
		t.Fatalf("SearchHubBundles() nextCursor = %q, want cur1", resp.Meta.NextCursor)
	}
}

func TestSearchHubBundlesNoAuthHeader(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("Authorization header = %q, want empty for public endpoint", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"data":[],"meta":{"hasMore":false}}`), nil
		}),
	}

	// Even with an API key, hub endpoints should not send auth.
	c := NewWithHTTPClient("https://example.test", "my-secret-key", clientHTTP)

	_, err := c.SearchHubBundles(t.Context(), "", "", "", 0, "")
	if err != nil {
		t.Fatalf("SearchHubBundles() error = %v", err)
	}
}

func TestSearchHubBundlesError(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return bundleJSONResponse(http.StatusInternalServerError, `{"error":"internal"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "", clientHTTP)

	_, err := c.SearchHubBundles(t.Context(), "test", "", "", 0, "")
	if err == nil {
		t.Fatal("SearchHubBundles() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("SearchHubBundles() error = %v, want status 500", err)
	}
}

func TestGetHubBundleDetailHappyPath(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/hub/bundles/acme/deploy-tool" {
				t.Fatalf("path = %q, want /v1/hub/bundles/acme/deploy-tool", r.URL.Path)
			}

			return bundleJSONResponse(http.StatusOK, `{
				"id":"b1","slug":"deploy-tool","displayName":"Deploy Tool",
				"bundleType":"tool","latestVersion":"1.0.0",
				"publisher":{"handle":"acme","trustTier":"verified"},
				"description":"A useful tool","loadCommand":"mush bundle load deploy-tool"
			}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "", clientHTTP)

	detail, err := c.GetHubBundleDetail(t.Context(), "acme", "deploy-tool")
	if err != nil {
		t.Fatalf("GetHubBundleDetail() error = %v", err)
	}

	if detail.Slug != "deploy-tool" {
		t.Fatalf("GetHubBundleDetail() slug = %q, want deploy-tool", detail.Slug)
	}

	if detail.Description != "A useful tool" {
		t.Fatalf("GetHubBundleDetail() description = %q, want 'A useful tool'", detail.Description)
	}

	if detail.Publisher.TrustTier != "verified" {
		t.Fatalf("GetHubBundleDetail() trustTier = %q, want verified", detail.Publisher.TrustTier)
	}
}

func TestGetHubBundleDetailNoAuthHeader(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("Authorization header = %q, want empty for public endpoint", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"id":"b1","slug":"test","publisher":{"handle":"pub"}}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "secret-key", clientHTTP)

	_, err := c.GetHubBundleDetail(t.Context(), "pub", "test")
	if err != nil {
		t.Fatalf("GetHubBundleDetail() error = %v", err)
	}
}

func TestGetHubBundleDetail404(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return bundleJSONResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "", clientHTTP)

	_, err := c.GetHubBundleDetail(t.Context(), "acme", "nonexistent")
	if err == nil {
		t.Fatal("GetHubBundleDetail() expected error for 404")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetHubBundleDetail() error = %v, want 'not found'", err)
	}
}

func TestListHubCategoriesHappyPath(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/v1/hub/categories" {
				t.Fatalf("path = %q, want /v1/hub/categories", r.URL.Path)
			}

			return bundleJSONResponse(http.StatusOK, `{"data":[
				{"slug":"agents","displayName":"Agents","bundleCount":10},
				{"slug":"tools","displayName":"Tools","bundleCount":25}
			]}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "", clientHTTP)

	cats, err := c.ListHubCategories(t.Context())
	if err != nil {
		t.Fatalf("ListHubCategories() error = %v", err)
	}

	if len(cats) != 2 {
		t.Fatalf("ListHubCategories() len = %d, want 2", len(cats))
	}

	if cats[0].Slug != "agents" {
		t.Fatalf("ListHubCategories()[0].Slug = %q, want agents", cats[0].Slug)
	}

	if cats[1].BundleCount != 25 {
		t.Fatalf("ListHubCategories()[1].BundleCount = %d, want 25", cats[1].BundleCount)
	}
}

func TestListHubCategoriesNoAuthHeader(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{
		Transport: bundleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("Authorization header = %q, want empty for public endpoint", got)
			}

			return bundleJSONResponse(http.StatusOK, `{"data":[]}`), nil
		}),
	}

	c := NewWithHTTPClient("https://example.test", "secret-key", clientHTTP)

	_, err := c.ListHubCategories(t.Context())
	if err != nil {
		t.Fatalf("ListHubCategories() error = %v", err)
	}
}
