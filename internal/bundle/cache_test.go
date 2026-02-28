package bundle

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
			if r.URL.Path != "/api/v1/bundles:resolve" {
				t.Fatalf("path = %q, want /api/v1/bundles:resolve", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"bundleId":"b1","versionId":"v1","version":"1.2.3","state":"published"}`,
				)),
			}, nil
		}),
	}

	c := client.NewWithHTTPClient("https://example.test", "test-key", clientHTTP)
	out := output.NewWriter(&bytes.Buffer{}, &bytes.Buffer{}, &terminal.Info{IsTTY: false})

	_, _, err := Pull(t.Context(), c, "workspace-1", "my-bundle", "", out)
	if err == nil {
		t.Fatal("Pull() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "did not include OCI reference or asset manifest metadata") {
		t.Fatalf("Pull() error = %v, want contract-gap message", err)
	}
}

func TestCleanStalePartials(t *testing.T) {
	parent := t.TempDir()
	cachePath := filepath.Join(parent, "1.0.0")

	// Create stale partial dirs.
	partial1 := filepath.Join(parent, "1.0.0.partial.abc123")
	partial2 := filepath.Join(parent, "1.0.0.partial.def456")

	if err := os.MkdirAll(partial1, 0o755); err != nil {
		t.Fatalf("MkdirAll partial1 error = %v", err)
	}

	if err := os.MkdirAll(partial2, 0o755); err != nil {
		t.Fatalf("MkdirAll partial2 error = %v", err)
	}

	// Create a file in one of the partials to verify recursive removal.
	if err := os.WriteFile(filepath.Join(partial1, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Create a non-partial dir that should not be removed.
	other := filepath.Join(parent, "2.0.0")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("MkdirAll other error = %v", err)
	}

	cleanStalePartials(cachePath)

	// Partials should be gone.
	if _, err := os.Stat(partial1); !os.IsNotExist(err) {
		t.Fatalf("partial1 still exists after cleanup")
	}

	if _, err := os.Stat(partial2); !os.IsNotExist(err) {
		t.Fatalf("partial2 still exists after cleanup")
	}

	// Non-partial dir should remain.
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-partial dir was incorrectly removed: %v", err)
	}
}

func TestPullCleanupOnFailure(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	// Simulate a resolve that returns assets but the asset fetch will fail.
	clientHTTP := &http.Client{
		Transport: cacheRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v1/bundles:resolve" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"bundleId":"b1","versionId":"v1","version":"1.2.3","state":"published","manifest":{"layers":[{"assetId":"a1","logicalPath":"skill.md","contentSha256":"abc123","sizeBytes":10}]}}`,
					)),
				}, nil
			}

			// Asset fetch fails.
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"server error"}`)),
			}, nil
		}),
	}

	c := client.NewWithHTTPClient("https://example.test", "test-key", clientHTTP)
	out := output.NewWriter(&bytes.Buffer{}, &bytes.Buffer{}, &terminal.Info{IsTTY: false})

	_, _, err := Pull(t.Context(), c, "workspace-1", "my-bundle", "", out)
	if err == nil {
		t.Fatal("Pull() expected error, got nil")
	}

	// Verify no partial staging directories remain.
	versionDir := filepath.Join(cacheRoot, "mush", "cache", "workspace-1", "my-bundle")

	entries, readErr := os.ReadDir(versionDir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return // parent dir doesn't exist, which is fine
		}

		t.Fatalf("ReadDir error = %v", readErr)
	}

	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial.") {
			t.Fatalf("staging directory not cleaned up: %s", e.Name())
		}
	}
}
