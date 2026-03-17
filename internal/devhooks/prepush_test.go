package devhooks

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestParsePushRefs(t *testing.T) {
	t.Parallel()

	refs, err := parsePushRefs(strings.NewReader("refs/heads/main abc refs/heads/main def\n"))
	if err != nil {
		t.Fatalf("parsePushRefs() error = %v", err)
	}

	want := []pushRef{{
		localRef:  "refs/heads/main",
		localSHA:  "abc",
		remoteRef: "refs/heads/main",
		remoteSHA: "def",
	}}

	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("parsePushRefs() = %#v, want %#v", refs, want)
	}
}

func TestCollectChangedFilesFallsBackForNewRemoteRef(t *testing.T) {
	t.Parallel()

	files, fallbackFull, err := collectChangedFiles(t.Context(), []pushRef{{
		localRef:  "refs/heads/feature",
		localSHA:  "abc",
		remoteRef: "refs/heads/feature",
		remoteSHA: zeroSHA,
	}}, func(context.Context, string, string) ([]string, error) {
		t.Fatal("gitDiff should not be called when the remote ref does not exist")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("collectChangedFiles() error = %v", err)
	}

	if len(files) != 0 {
		t.Fatalf("collectChangedFiles() files = %v, want empty", files)
	}

	if !fallbackFull {
		t.Fatal("collectChangedFiles() fallbackFull = false, want true")
	}
}

func TestClassifyChanges(t *testing.T) {
	t.Parallel()

	changes := classifyChanges([]string{
		"cmd/mush/main.go",
		"cmd/mush/root.go",
		"install.sh",
		".github/workflows/ci.yml",
		"go.mod",
		"test/install/install_test.go",
	})

	if !reflect.DeepEqual(changes.goFiles, []string{"cmd/mush/main.go", "cmd/mush/root.go", "test/install/install_test.go"}) {
		t.Fatalf("goFiles = %v", changes.goFiles)
	}

	if !reflect.DeepEqual(changes.lintPackages, []string{"./cmd/mush/...", "./test/install/..."}) {
		t.Fatalf("lintPackages = %v", changes.lintPackages)
	}

	if !reflect.DeepEqual(changes.testPackages, []string{"./cmd/mush", "./test/install"}) {
		t.Fatalf("testPackages = %v", changes.testPackages)
	}

	if !reflect.DeepEqual(changes.shellFiles, []string{"install.sh"}) {
		t.Fatalf("shellFiles = %v", changes.shellFiles)
	}

	if !reflect.DeepEqual(changes.workflowFiles, []string{".github/workflows/ci.yml"}) {
		t.Fatalf("workflowFiles = %v", changes.workflowFiles)
	}

	if !changes.modChanged {
		t.Fatal("modChanged = false, want true")
	}

	if !changes.installTests {
		t.Fatal("installTests = false, want true")
	}
}

func TestRunPrePushRunsTargetedTasks(t *testing.T) {
	t.Parallel()

	var ran []string

	err := RunPrePush(t.Context(), strings.NewReader("refs/heads/main abc refs/heads/main def\n"),
		func(_ context.Context, name string, args ...string) error {
			ran = append(ran, strings.Join(append([]string{name}, args...), " "))
			return nil
		},
		func(context.Context, string, string) ([]string, error) {
			return []string{"cmd/mush/main.go", ".github/workflows/ci.yml", "install.sh"}, nil
		},
	)
	if err != nil {
		t.Fatalf("RunPrePush() error = %v", err)
	}

	want := []string{
		"check:fmt:files -- cmd/mush/main.go",
		"check:lint:files -- ./cmd/mush/...",
		"check:policy",
		"check:shell:files -- install.sh",
		"check:workflow:files -- .github/workflows/ci.yml",
		"check:install",
		"check:test:packages -- ./cmd/mush",
	}

	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("RunPrePush() tasks = %v, want %v", ran, want)
	}
}

func TestRunPrePushFallsBackToFullChecks(t *testing.T) {
	t.Parallel()

	var ran []string

	err := RunPrePush(t.Context(), strings.NewReader("refs/heads/main abc refs/heads/main "+zeroSHA+"\n"),
		func(_ context.Context, name string, _ ...string) error {
			ran = append(ran, name)
			return nil
		},
		func(context.Context, string, string) ([]string, error) {
			t.Fatal("gitDiff should not be called during fallback")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("RunPrePush() error = %v", err)
	}

	want := []string{"check:lint", "check:policy", "check:test"}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("RunPrePush() tasks = %v, want %v", ran, want)
	}
}
