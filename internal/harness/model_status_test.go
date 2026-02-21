//go:build unix

package harness

import (
	"strings"
	"testing"
)

func TestRenderStatusIncludesStartingAndReadyLabels(t *testing.T) {
	if got := renderStatus(StatusStarting); !strings.Contains(got, "Starting") {
		t.Fatalf("renderStatus(StatusStarting) = %q, want label containing Starting", got)
	}

	if got := renderStatus(StatusReady); !strings.Contains(got, "Ready") {
		t.Fatalf("renderStatus(StatusReady) = %q, want label containing Ready", got)
	}
}
