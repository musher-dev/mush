package bundle

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

type cacheRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cacheRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPullFailsWhenResolveLacksDownloadMetadata(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	clientHTTP := &http.Client{
		Transport: cacheRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "/assets"):
				// Return empty assets list — no layers in manifest.
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
				}, nil
			default:
				// Hub detail response.
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"b1","slug":"my-bundle","latestVersion":"1.2.3","publisher":{"handle":"acme"}}`,
					)),
				}, nil
			}
		}),
	}

	c := client.NewWithHTTPClient("https://example.test", "test-key", clientHTTP)
	out := output.NewWriter(&bytes.Buffer{}, &bytes.Buffer{}, &terminal.Info{IsTTY: false})

	_, err := Pull(t.Context(), c, "acme", "my-bundle", "", out)
	if err == nil {
		t.Fatal("Pull() expected error, got nil")
	}

	if !errors.Is(err, ErrNoAssets) {
		t.Fatalf("Pull() error = %v, want ErrNoAssets", err)
	}
}

func TestPullCleanupOnFailure(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	// Simulate a resolve that returns assets but the asset fetch will fail.
	clientHTTP := &http.Client{
		Transport: cacheRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "/assets"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"a1","assetType":"skill","logicalPath":"skill.md","contentSha256":"abc123","sizeBytes":10}]}`)),
				}, nil
			case strings.HasPrefix(r.URL.Path, "/v1/hub/bundles/"):
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"b1","slug":"my-bundle","latestVersion":"1.2.3","publisher":{"handle":"acme"}}`,
					)),
				}, nil
			default:
				// Asset fetch via runner endpoint fails.
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":"server error"}`)),
				}, nil
			}
		}),
	}

	c := client.NewWithHTTPClient("https://example.test", "test-key", clientHTTP)
	out := output.NewWriter(&bytes.Buffer{}, &bytes.Buffer{}, &terminal.Info{IsTTY: false})

	_, err := Pull(t.Context(), c, "acme", "my-bundle", "", out)
	if err == nil {
		t.Fatal("Pull() expected error, got nil")
	}
}
