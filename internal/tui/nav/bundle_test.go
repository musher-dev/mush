package nav

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBundleInputScreen(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Press 'b' to go to bundle input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Fatalf("activeScreen = %d, want screenBundleInput", mdl.activeScreen)
	}

	view := mdl.View()
	if !strings.Contains(view, "Run harness") {
		t.Error("bundle input view should contain 'Run harness'")
	}

	if !strings.Contains(view, "claude") {
		t.Error("bundle input view should contain 'claude' harness option")
	}

	if !strings.Contains(view, "codex") {
		t.Error("bundle input view should contain 'codex' harness option")
	}
}

func TestBundleInputEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Go to bundle input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Fatalf("expected bundle input screen")
	}

	// Esc goes back to home.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc", mdl.activeScreen)
	}
}

func TestBundleInputTabSwitchesFocus(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Go to bundle input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.bundleInput.focusArea != bundleFocusInput {
		t.Error("should start with focus on input")
	}

	// Tab switches to list.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleInput.focusArea != bundleFocusList {
		t.Error("after tab, focus should be on list")
	}

	// Tab again switches to harness.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleInput.focusArea != bundleFocusHarness {
		t.Error("after second tab, focus should be on harness")
	}

	// Tab once more returns to input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleInput.focusArea != bundleFocusInput {
		t.Error("after third tab, focus should be back on input")
	}
}

func TestBundleInputHarnessNavigation(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Go to bundle input and switch to harness panel (tab twice: input→list→harness).
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleInput.focusArea != bundleFocusHarness {
		t.Fatalf("focusArea = %d, want bundleFocusHarness", mdl.bundleInput.focusArea)
	}

	if mdl.bundleInput.harnessCur != 0 {
		t.Errorf("harnessCur = %d, want 0", mdl.bundleInput.harnessCur)
	}

	if len(mdl.harnesses) < 2 {
		t.Fatalf("need at least 2 harnesses, got %d", len(mdl.harnesses))
	}

	// Move down.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.bundleInput.harnessCur != 1 {
		t.Errorf("harnessCur = %d, want 1 after down", mdl.bundleInput.harnessCur)
	}

	// Move down until we hit the end, then verify it clamps.
	last := len(mdl.harnesses) - 1
	for mdl.bundleInput.harnessCur < last {
		mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.bundleInput.harnessCur != last {
		t.Errorf("harnessCur = %d, want %d (clamped)", mdl.bundleInput.harnessCur, last)
	}

	// Move up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.bundleInput.harnessCur != last-1 {
		t.Errorf("harnessCur = %d, want %d after up", mdl.bundleInput.harnessCur, last-1)
	}
}

func TestBundleInputEmptySubmitShowsError(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Go to bundle input with empty text.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Submit empty.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError for empty input", mdl.activeScreen)
	}
}

func TestBundleInputNoClientShowsError(t *testing.T) {
	t.Parallel()

	mdl := newModel(t.Context(), &Dependencies{})

	// Go to bundle input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Type a valid namespace/slug.
	for _, r := range "acme/test-bundle" {
		mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Submit.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError (no client)", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "Unable to connect") {
		t.Errorf("error message = %q, want to contain 'Unable to connect'", mdl.bundleError.message)
	}
}

func TestBundleResolvedMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleResolving

	msg := bundleResolvedMsg{
		slug:       "test-bundle",
		version:    "1.0.0",
		assetCount: 3,
		harness:    "claude",
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleConfirm {
		t.Errorf("activeScreen = %d, want screenBundleConfirm", mdl.activeScreen)
	}

	if mdl.bundleConfirm.slug != "test-bundle" {
		t.Errorf("confirm slug = %q, want 'test-bundle'", mdl.bundleConfirm.slug)
	}

	if mdl.bundleConfirm.version != "1.0.0" {
		t.Errorf("confirm version = %q, want '1.0.0'", mdl.bundleConfirm.version)
	}

	if mdl.bundleConfirm.assetCount != 3 {
		t.Errorf("confirm assetCount = %d, want 3", mdl.bundleConfirm.assetCount)
	}
}

func TestBundleResolveErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleResolving

	msg := bundleResolveErrorMsg{
		err:     fmt.Errorf("not found"),
		slug:    "bad-bundle",
		version: "",
		harness: "claude",
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "not found") {
		t.Errorf("error message = %q, want to contain 'not found'", mdl.bundleError.message)
	}
}

func TestBundleConfirmTabSwitchesButtons(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleConfirm
	mdl.bundleConfirm = bundleConfirmState{
		slug:       "test",
		version:    "1.0.0",
		assetCount: 1,
		harness:    "claude",
		buttonIdx:  0,
	}

	// Tab switches from Load to Cancel.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleConfirm.buttonIdx != 1 {
		t.Errorf("buttonIdx = %d, want 1 after tab", mdl.bundleConfirm.buttonIdx)
	}

	// Tab again back to Load.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleConfirm.buttonIdx != 0 {
		t.Errorf("buttonIdx = %d, want 0 after second tab", mdl.bundleConfirm.buttonIdx)
	}
}

func TestBundleConfirmCancelGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleConfirm)
	mdl.bundleConfirm = bundleConfirmState{
		slug:      "test",
		version:   "1.0.0",
		harness:   "claude",
		buttonIdx: 1, // Cancel button focused
	}

	// Enter on Cancel goes back.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after cancel", mdl.activeScreen)
	}
}

func TestBundleCacheHitMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleProgress
	mdl.bundleProgress.slug = "test"
	mdl.bundleProgress.version = "1.0.0"

	msg := bundleCacheHitMsg{
		cachePath: "/tmp/cache/test/1.0.0",
		harness:   "claude",
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleComplete {
		t.Errorf("activeScreen = %d, want screenBundleComplete", mdl.activeScreen)
	}

	if mdl.bundleComplete.cachePath != "/tmp/cache/test/1.0.0" {
		t.Errorf("cachePath = %q, want '/tmp/cache/test/1.0.0'", mdl.bundleComplete.cachePath)
	}
}

func TestBundleCompleteEscGoesHome(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleComplete)
	mdl.bundleComplete = bundleCompleteState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		harness:   "claude",
		cachePath: "/tmp/test",
	}

	// Esc goes home.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc", mdl.activeScreen)
	}
}

func TestBundleCompleteLaunchSetsResult(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleComplete)
	mdl.bundleComplete = bundleCompleteState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		harness:   "claude",
		cachePath: "/tmp/test",
	}

	// Enter launches (sets result and quits).
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.result == nil {
		t.Fatal("result should be set after launch")
	}

	if mdl.result.Action != ActionBundleLoad {
		t.Errorf("result.Action = %d, want ActionBundleLoad", mdl.result.Action)
	}

	if mdl.result.BundleSlug != "test" {
		t.Errorf("result.BundleSlug = %q, want 'test'", mdl.result.BundleSlug)
	}

	if mdl.result.BundleNamespace != "acme" {
		t.Errorf("result.BundleNamespace = %q, want 'acme'", mdl.result.BundleNamespace)
	}

	// Should have a quit command.
	if cmd == nil {
		t.Fatal("expected quit command")
	}

	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
	}
}

func TestBundleErrorRetry(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleError
	mdl.bundleError = bundleErrorState{
		message: "not found",
		slug:    "test-bundle",
		version: "1.0.0",
		harness: "claude",
	}

	// 'r' should attempt retry — but without a client it stays on error.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Without deps/client, retry is a no-op.
	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError (no client for retry)", mdl.activeScreen)
	}
}

func TestBundleErrorEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleError)
	mdl.bundleError = bundleErrorState{
		message: "error",
		slug:    "test",
	}

	// Esc goes back.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc from error", mdl.activeScreen)
	}
}

func TestBundleDownloadProgressMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleProgress

	msg := bundleDownloadProgressMsg{
		current: 2,
		total:   5,
		label:   "Downloading asset 2/5",
	}

	mdl = updateModel(mdl, msg)

	if mdl.bundleProgress.current != 2 {
		t.Errorf("progress current = %d, want 2", mdl.bundleProgress.current)
	}

	if mdl.bundleProgress.total != 5 {
		t.Errorf("progress total = %d, want 5", mdl.bundleProgress.total)
	}
}

func TestBundleDownloadErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenBundleProgress
	mdl.bundleProgress.slug = "test"
	mdl.bundleProgress.version = "1.0.0"

	msg := bundleDownloadErrorMsg{
		err:     fmt.Errorf("network error"),
		harness: "claude",
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "network error") {
		t.Errorf("error message = %q, want to contain 'network error'", mdl.bundleError.message)
	}
}

func TestBundleInputTwoPanelView(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Simulate a wide terminal to trigger two-panel mode.
	mdl = updateModel(mdl, tea.WindowSizeMsg{Width: 130, Height: 40})

	// Navigate to bundle input screen.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Fatalf("activeScreen = %d, want screenBundleInput", mdl.activeScreen)
	}

	view := mdl.View()
	if !strings.Contains(view, "Run harness") {
		t.Error("two-panel view should contain 'Run harness' panel title")
	}

	if !strings.Contains(view, "Harness") {
		t.Error("two-panel view should contain 'Harness' panel title")
	}

	if !strings.Contains(view, "namespace/slug") {
		t.Error("two-panel view should contain slug input placeholder")
	}

	if !strings.Contains(view, "Find a bundle on the Hub") {
		t.Error("two-panel view should contain 'Find a bundle on the Hub' action link")
	}

	if !strings.Contains(view, "switch panel") {
		t.Error("two-panel footer should contain 'switch panel' hint")
	}
}

func TestBundleScreenViews(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		screen screen
		setup  func(*model)
		check  string
	}{
		{
			name:   "resolving",
			screen: screenBundleResolving,
			setup: func(m *model) {
				m.bundleResolve.slug = "test"
			},
			check: "Resolving",
		},
		{
			name:   "confirm",
			screen: screenBundleConfirm,
			setup: func(m *model) {
				m.bundleConfirm = bundleConfirmState{
					slug: "test", version: "1.0.0", assetCount: 2, harness: "claude",
				}
			},
			check: "Confirm",
		},
		{
			name:   "progress",
			screen: screenBundleProgress,
			setup: func(m *model) {
				m.bundleProgress.slug = "test"
				m.bundleProgress.version = "1.0.0"
				m.bundleProgress.total = 3
			},
			check: "Downloading",
		},
		{
			name:   "complete",
			screen: screenBundleComplete,
			setup: func(m *model) {
				m.bundleComplete = bundleCompleteState{
					slug: "test", version: "1.0.0", cachePath: "/tmp/test",
				}
			},
			check: "ready",
		},
		{
			name:   "error",
			screen: screenBundleError,
			setup: func(m *model) {
				m.bundleError = bundleErrorState{message: "something failed"}
			},
			check: "Error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mdl := testModel()
			mdl.activeScreen = test.screen
			test.setup(mdl)

			view := mdl.View()
			if !strings.Contains(view, test.check) {
				t.Errorf("view for %s should contain %q", test.name, test.check)
			}
		})
	}
}
