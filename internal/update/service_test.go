package update

import (
	"errors"
	"testing"
)

func TestCurrentInstallContext_UnknownWhenExecutableUnavailable(t *testing.T) {
	prev := executablePath
	executablePath = func() (string, error) {
		return "", errors.New("boom")
	}

	t.Cleanup(func() { executablePath = prev })

	ctx := CurrentInstallContext()
	if ctx.ExecPathKnown {
		t.Fatal("ExecPathKnown = true, want false")
	}

	if ctx.Source != InstallSourceUnknown {
		t.Fatalf("Source = %q, want %q", ctx.Source, InstallSourceUnknown)
	}
}

func TestCurrentInstallContext_Homebrew(t *testing.T) {
	prev := executablePath
	executablePath = func() (string, error) {
		return "/opt/homebrew/Cellar/mush/1.2.3/bin/mush", nil
	}

	t.Cleanup(func() { executablePath = prev })

	ctx := CurrentInstallContext()
	if !ctx.ExecPathKnown {
		t.Fatal("ExecPathKnown = false, want true")
	}

	if ctx.Source != InstallSourceHomebrew {
		t.Fatalf("Source = %q, want %q", ctx.Source, InstallSourceHomebrew)
	}
}
