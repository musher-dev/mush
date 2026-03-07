package nav

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

func TestBundleInputScreen(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Press 'r' to go to bundle input.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if mdl.activeScreen != screenBundleInput {
		t.Fatalf("activeScreen = %d, want screenBundleInput", mdl.activeScreen)
	}

	view := mdl.View()
	if !strings.Contains(view, "Load bundle") {
		t.Error("bundle input view should contain 'Load bundle'")
	}

	if !strings.Contains(view, "namespace/slug") {
		t.Error("bundle input view should contain slug input placeholder")
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

	// Tab again returns to input (only 2 focus areas now).
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.bundleInput.focusArea != bundleFocusInput {
		t.Error("after second tab, focus should be back on input")
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
	}

	mdl = updateModel(mdl, msg)

	// Resolve now skips confirm and goes directly to progress (download/cache check).
	if mdl.activeScreen != screenBundleProgress {
		t.Errorf("activeScreen = %d, want screenBundleProgress", mdl.activeScreen)
	}

	if mdl.bundleProgress.slug != "test-bundle" {
		t.Errorf("progress slug = %q, want 'test-bundle'", mdl.bundleProgress.slug)
	}

	if mdl.bundleProgress.version != "1.0.0" {
		t.Errorf("progress version = %q, want '1.0.0'", mdl.bundleProgress.version)
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
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "not found") {
		t.Errorf("error message = %q, want to contain 'not found'", mdl.bundleError.message)
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
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleAction {
		t.Errorf("activeScreen = %d, want screenBundleAction", mdl.activeScreen)
	}

	if mdl.bundleAction.cachePath != "/tmp/cache/test/1.0.0" {
		t.Errorf("cachePath = %q, want '/tmp/cache/test/1.0.0'", mdl.bundleAction.cachePath)
	}
}

func TestBundleActionEscGoesHome(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleAction)
	mdl.bundleAction = bundleActionState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		cachePath: "/tmp/test",
	}

	// Esc goes home.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc", mdl.activeScreen)
	}
}

func TestBundleActionRunPushesHarness(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleAction)
	mdl.bundleAction = bundleActionState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		cachePath: "/tmp/test",
		buttonIdx: 0, // Run
	}

	// Enter selects Run → pushes harness screen.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenBundleHarness {
		t.Errorf("activeScreen = %d, want screenBundleHarness", mdl.activeScreen)
	}

	if mdl.bundleHarness.forInstall {
		t.Error("forInstall should be false for Run action")
	}
}

func TestBundleActionInstallPushesHarness(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleAction)
	mdl.bundleAction = bundleActionState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		cachePath: "/tmp/test",
		buttonIdx: 1, // Install
	}

	// Enter selects Install → pushes harness screen.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenBundleHarness {
		t.Errorf("activeScreen = %d, want screenBundleHarness", mdl.activeScreen)
	}

	if !mdl.bundleHarness.forInstall {
		t.Error("forInstall should be true for Install action")
	}
}

func TestBundleHarnessSelectRun(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleHarness)

	// Build installed indices for the test harnesses.
	var installed []int
	for i := range mdl.harnesses {
		installed = append(installed, i)
	}

	mdl.bundleHarness = bundleHarnessState{
		namespace: "acme",
		slug:      "test",
		version:   "1.0.0",
		cachePath: "/tmp/test",
		installed: installed,
	}

	// Enter selects harness → exits with ActionBundleLoad.
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.result == nil {
		t.Fatal("result should be set after harness selection")
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

func TestBundleHarnessSelectInstall(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleHarness)

	var installed []int
	for i := range mdl.harnesses {
		installed = append(installed, i)
	}

	mdl.bundleHarness = bundleHarnessState{
		namespace:  "acme",
		slug:       "test",
		version:    "1.0.0",
		cachePath:  "/tmp/test",
		installed:  installed,
		forInstall: true,
	}

	// Enter selects harness → pushes install confirm screen.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenBundleInstallConfirm {
		t.Errorf("activeScreen = %d, want screenBundleInstallConfirm", mdl.activeScreen)
	}

	if mdl.bundleInstallConfirm.namespace != "acme" {
		t.Errorf("namespace = %q, want 'acme'", mdl.bundleInstallConfirm.namespace)
	}
}

func TestBundleInstallConfirmWithoutConflicts(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleInstallConfirm)
	mdl.bundleInstallConfirm = bundleInstallConfirmState{
		namespace:    "acme",
		slug:         "test",
		version:      "1.0.0",
		cachePath:    "/tmp/test",
		harness:      "claude",
		hasConflicts: false,
		buttonIdx:    0, // Install button
	}

	// Enter on Install → exits with ActionBundleInstall.
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.result == nil {
		t.Fatal("result should be set after install")
	}

	if mdl.result.Action != ActionBundleInstall {
		t.Errorf("result.Action = %d, want ActionBundleInstall", mdl.result.Action)
	}

	if mdl.result.Force {
		t.Error("force should be false without conflicts")
	}

	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestBundleInstallConfirmWithConflicts(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenBundleInstallConfirm)
	mdl.bundleInstallConfirm = bundleInstallConfirmState{
		namespace:     "acme",
		slug:          "test",
		version:       "1.0.0",
		cachePath:     "/tmp/test",
		harness:       "claude",
		hasConflicts:  true,
		conflictPaths: []string{"/path/to/conflict"},
		buttonIdx:     1, // Install button (shifted due to toggle at 0)
	}

	// Enter on Install → exits with ActionBundleInstall (force=false).
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.result == nil {
		t.Fatal("result should be set after install")
	}

	if mdl.result.Action != ActionBundleInstall {
		t.Errorf("result.Action = %d, want ActionBundleInstall", mdl.result.Action)
	}

	if mdl.result.Force {
		t.Error("force should be false when toggle not checked")
	}

	if cmd == nil {
		t.Fatal("expected quit command")
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

func TestBundleActionScreenFromSeed(t *testing.T) {
	t.Parallel()

	deps := &Dependencies{
		InitialBundle: &BundleSeed{
			Namespace: "acme",
			Slug:      "seeded-kit",
			Version:   "2.0.0",
			CachePath: "/tmp/seeded",
		},
	}

	mdl := newModel(t.Context(), deps)

	if mdl.activeScreen != screenBundleAction {
		t.Fatalf("activeScreen = %d, want screenBundleAction", mdl.activeScreen)
	}

	if mdl.bundleAction.namespace != "acme" {
		t.Errorf("namespace = %q, want 'acme'", mdl.bundleAction.namespace)
	}

	if mdl.bundleAction.slug != "seeded-kit" {
		t.Errorf("slug = %q, want 'seeded-kit'", mdl.bundleAction.slug)
	}

	if mdl.bundleAction.version != "2.0.0" {
		t.Errorf("version = %q, want '2.0.0'", mdl.bundleAction.version)
	}

	if mdl.bundleAction.cachePath != "/tmp/seeded" {
		t.Errorf("cachePath = %q, want '/tmp/seeded'", mdl.bundleAction.cachePath)
	}

	if len(mdl.screenStack) != 1 || mdl.screenStack[0] != screenHome {
		t.Errorf("screenStack = %v, want [screenHome]", mdl.screenStack)
	}
}

func TestBundleActionScreenSeedEscGoesHome(t *testing.T) {
	t.Parallel()

	deps := &Dependencies{
		InitialBundle: &BundleSeed{
			Namespace: "acme",
			Slug:      "seeded-kit",
			Version:   "2.0.0",
			CachePath: "/tmp/seeded",
		},
	}

	mdl := newModel(t.Context(), deps)

	if mdl.activeScreen != screenBundleAction {
		t.Fatalf("expected bundle action screen from seed")
	}

	// Esc should return to home.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc", mdl.activeScreen)
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
		err: fmt.Errorf("network error"),
	}

	mdl = updateModel(mdl, msg)

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "network error") {
		t.Errorf("error message = %q, want to contain 'network error'", mdl.bundleError.message)
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
			name:   "action",
			screen: screenBundleAction,
			setup: func(m *model) {
				m.bundleAction = bundleActionState{
					slug: "test", version: "1.0.0", cachePath: "/tmp/test",
				}
			},
			check: "Contents",
		},
		{
			name:   "harness",
			screen: screenBundleHarness,
			setup: func(m *model) {
				var installed []int
				for i := range m.harnesses {
					installed = append(installed, i)
				}

				m.bundleHarness = bundleHarnessState{
					slug: "test", version: "1.0.0", installed: installed,
				}
			},
			check: "Harness",
		},
		{
			name:   "install_confirm",
			screen: screenBundleInstallConfirm,
			setup: func(m *model) {
				m.bundleInstallConfirm = bundleInstallConfirmState{
					slug: "test", version: "1.0.0", harness: "claude",
				}
			},
			check: "Install",
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

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1843, "1.8 KB"},
		{1048576, "1.0 MB"},
		{2200000, "2.1 MB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGroupLayers(t *testing.T) {
	t.Parallel()

	layers := []client.BundleLayer{
		{AssetType: "skill", LogicalPath: "skills/web/SKILL.md", SizeBytes: 1024},
		{AssetType: "skill", LogicalPath: "skills/api/SKILL.md", SizeBytes: 800},
		{AssetType: "agent_definition", LogicalPath: "agents/tools.yaml", SizeBytes: 420},
		{AssetType: "tool_config", LogicalPath: "tools/mcp.json", SizeBytes: 2100},
	}

	groups := groupLayers(layers)

	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}

	// Check ordering: Skills, Agents, Tools.
	if groups[0].label != "Skills" {
		t.Errorf("groups[0].label = %q, want Skills", groups[0].label)
	}

	if groups[1].label != "Agents" {
		t.Errorf("groups[1].label = %q, want Agents", groups[1].label)
	}

	if groups[2].label != "Tools" {
		t.Errorf("groups[2].label = %q, want Tools", groups[2].label)
	}

	// Check aggregation.
	if groups[0].count != 2 {
		t.Errorf("Skills count = %d, want 2", groups[0].count)
	}

	if groups[0].size != 1824 {
		t.Errorf("Skills size = %d, want 1824", groups[0].size)
	}

	if len(groups[0].paths) != 2 {
		t.Errorf("Skills paths len = %d, want 2", len(groups[0].paths))
	}
}

func TestGroupLayersEmpty(t *testing.T) {
	t.Parallel()

	groups := groupLayers(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil layers, got %d", len(groups))
	}
}

func TestRenderBundleContentsEmpty(t *testing.T) {
	t.Parallel()

	styles := newTheme(80)
	result := renderBundleContents(&styles, nil, layoutSingle, 60)

	if !strings.Contains(result, "Contents") {
		t.Error("should contain 'Contents' title")
	}

	if !strings.Contains(result, "No assets") {
		t.Error("should contain 'No assets' placeholder")
	}
}

func TestRenderBundleContentsSingleType(t *testing.T) {
	t.Parallel()

	styles := newTheme(80)
	layers := []client.BundleLayer{
		{AssetType: "skill", LogicalPath: "skills/web/SKILL.md", SizeBytes: 1024},
	}

	result := renderBundleContents(&styles, layers, layoutSingle, 60)

	if !strings.Contains(result, "Skills (1)") {
		t.Error("should contain 'Skills (1)'")
	}

	if !strings.Contains(result, "skills/web/SKILL.md") {
		t.Error("should contain the file path")
	}
}

func TestRenderBundleContentsMultiType(t *testing.T) {
	t.Parallel()

	styles := newTheme(80)
	layers := []client.BundleLayer{
		{AssetType: "skill", LogicalPath: "skills/web/SKILL.md", SizeBytes: 1024},
		{AssetType: "agent_definition", LogicalPath: "agents/tools.yaml", SizeBytes: 420},
		{AssetType: "tool_config", LogicalPath: "tools/mcp.json", SizeBytes: 2100},
	}

	result := renderBundleContents(&styles, layers, layoutSingle, 60)

	if !strings.Contains(result, "Skills (1)") {
		t.Error("should contain 'Skills (1)'")
	}

	if !strings.Contains(result, "Agents (1)") {
		t.Error("should contain 'Agents (1)'")
	}

	if !strings.Contains(result, "Tools (1)") {
		t.Error("should contain 'Tools (1)'")
	}
}

func TestRenderBundleContentsTruncation(t *testing.T) {
	t.Parallel()

	styles := newTheme(80)
	layers := []client.BundleLayer{
		{AssetType: "skill", LogicalPath: "skills/a/SKILL.md", SizeBytes: 100},
		{AssetType: "skill", LogicalPath: "skills/b/SKILL.md", SizeBytes: 100},
		{AssetType: "skill", LogicalPath: "skills/c/SKILL.md", SizeBytes: 100},
		{AssetType: "skill", LogicalPath: "skills/d/SKILL.md", SizeBytes: 100},
	}

	result := renderBundleContents(&styles, layers, layoutSingle, 60)

	if !strings.Contains(result, "Skills (4)") {
		t.Error("should contain 'Skills (4)'")
	}

	if !strings.Contains(result, "+2 more") {
		t.Error("should contain '+2 more' truncation line")
	}
}

func TestRenderBundleContentsCompactNoPaths(t *testing.T) {
	t.Parallel()

	styles := newTheme(50)
	layers := []client.BundleLayer{
		{AssetType: "skill", LogicalPath: "skills/web/SKILL.md", SizeBytes: 1024},
	}

	result := renderBundleContents(&styles, layers, layoutCompact, 50)

	if !strings.Contains(result, "Skills (1)") {
		t.Error("should contain group header")
	}

	if strings.Contains(result, "skills/web/SKILL.md") {
		t.Error("compact layout should not contain file paths")
	}
}
