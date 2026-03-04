package nav

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHyperlink(t *testing.T) {
	t.Parallel()

	result := hyperlink("https://example.com", "Example")

	if !strings.Contains(result, "Example") {
		t.Error("hyperlink output should contain the visible text")
	}

	if !strings.Contains(result, "https://example.com") {
		t.Error("hyperlink output should contain the URL")
	}

	if w := ansi.StringWidth(result); w != len("Example") {
		t.Errorf("StringWidth = %d, want %d", w, len("Example"))
	}
}

func TestHyperlinkWithStyledText(t *testing.T) {
	t.Parallel()

	styled := "\x1b[1mBold\x1b[0m"
	result := hyperlink("https://example.com", styled)

	if !strings.Contains(result, "Bold") {
		t.Error("hyperlink should preserve inner styled text")
	}

	if w := ansi.StringWidth(result); w != len("Bold") {
		t.Errorf("StringWidth = %d, want %d", w, len("Bold"))
	}
}
