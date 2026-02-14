//go:build unix

package harness

import "testing"

func TestSummarizeMCPServers(t *testing.T) {
	if got := summarizeMCPServers(nil); got != "none" {
		t.Fatalf("summarize(nil) = %q, want none", got)
	}
	if got := summarizeMCPServers([]string{"linear"}); got != "linear" {
		t.Fatalf("summarize(single) = %q, want linear", got)
	}
	if got := summarizeMCPServers([]string{"linear", "github"}); got != "linear, github" {
		t.Fatalf("summarize(multi) = %q, want 'linear, github'", got)
	}
}

func TestSameStringSlice(t *testing.T) {
	if !sameStringSlice([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatalf("expected sameStringSlice to be order-insensitive")
	}
	if sameStringSlice([]string{"a"}, []string{"a", "b"}) {
		t.Fatalf("expected different lengths to be false")
	}
	if sameStringSlice([]string{"a", "c"}, []string{"a", "b"}) {
		t.Fatalf("expected different values to be false")
	}
}
