//go:build unix

package harness

import (
	"testing"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

func TestSummarizeMCPServers(t *testing.T) {
	if got := harnesstype.SummarizeMCPServers(nil); got != "none" {
		t.Fatalf("summarize(nil) = %q, want none", got)
	}

	if got := harnesstype.SummarizeMCPServers([]string{"linear"}); got != "linear" {
		t.Fatalf("summarize(single) = %q, want linear", got)
	}

	if got := harnesstype.SummarizeMCPServers([]string{"linear", "github"}); got != "linear, github" {
		t.Fatalf("summarize(multi) = %q, want 'linear, github'", got)
	}
}

func TestSameStringSlice(t *testing.T) {
	if !harnesstype.SameStringSlice([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatalf("expected sameStringSlice to be order-insensitive")
	}

	if harnesstype.SameStringSlice([]string{"a"}, []string{"a", "b"}) {
		t.Fatalf("expected different lengths to be false")
	}

	if harnesstype.SameStringSlice([]string{"a", "c"}, []string{"a", "b"}) {
		t.Fatalf("expected different values to be false")
	}
}
