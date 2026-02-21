package bundle

import (
	"strings"
	"testing"
)

func TestMergeJSONDocs(t *testing.T) {
	existing := []byte(`{"mcpServers":{"alpha":{"command":"a"}}}`)
	docs := [][]byte{
		[]byte(`{"mcpServers":{"beta":{"command":"b"}}}`),
		[]byte(`{"mcpServers":{"alpha":{"args":["--x"]}}}`),
	}

	got, err := MergeJSONDocs(existing, docs)
	if err != nil {
		t.Fatalf("MergeJSONDocs() error = %v", err)
	}

	s := string(got)
	if !strings.Contains(s, `"alpha"`) || !strings.Contains(s, `"beta"`) {
		t.Fatalf("merged json missing expected keys: %s", s)
	}
}

func TestMergeTOMLDocs(t *testing.T) {
	existing := []byte("[mcp_servers.alpha]\ncommand = \"a\"\n")
	docs := [][]byte{
		[]byte("[mcp_servers.beta]\ncommand = \"b\"\n"),
	}

	got, err := MergeTOMLDocs(existing, docs)
	if err != nil {
		t.Fatalf("MergeTOMLDocs() error = %v", err)
	}

	s := string(got)
	if !strings.Contains(s, "[mcp_servers.alpha]") || !strings.Contains(s, "[mcp_servers.beta]") {
		t.Fatalf("merged toml missing expected sections: %s", s)
	}
}
