package bundle

import (
	"bytes"
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

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
