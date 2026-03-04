package nav

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

func TestHubExploreFromHome(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// 'e' should go to hub explore.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if mdl.activeScreen != screenHubExplore {
		t.Errorf("activeScreen = %d, want screenHubExplore", mdl.activeScreen)
	}

	if !mdl.hubExplore.loading {
		t.Error("hubExplore.loading should be true after activation")
	}
}

func TestHubExploreEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()

	// Go to hub explore.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if mdl.activeScreen != screenHubExplore {
		t.Fatalf("expected hub explore screen")
	}

	// Esc goes back to home.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc", mdl.activeScreen)
	}
}

func TestHubExploreTabCyclesFocus(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if mdl.hubExplore.focusArea != 0 {
		t.Fatalf("focusArea = %d, want 0 (search)", mdl.hubExplore.focusArea)
	}

	// Tab cycles to categories.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.hubExplore.focusArea != 1 {
		t.Errorf("focusArea = %d, want 1 (categories)", mdl.hubExplore.focusArea)
	}

	// Tab cycles to list.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.hubExplore.focusArea != 2 {
		t.Errorf("focusArea = %d, want 2 (list)", mdl.hubExplore.focusArea)
	}

	// Tab wraps back to search.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyTab})

	if mdl.hubExplore.focusArea != 0 {
		t.Errorf("focusArea = %d, want 0 (search) after wrap", mdl.hubExplore.focusArea)
	}
}

func TestHubSearchResultPopulatesList(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.loading = true
	mdl.hubExplore.searchID = 1

	msg := hubSearchResultMsg{
		results: []client.HubBundleSummary{
			{Slug: "bundle-a", DisplayName: "Bundle A", Publisher: client.HubPublisher{Handle: "pub"}},
			{Slug: "bundle-b", DisplayName: "Bundle B", Publisher: client.HubPublisher{Handle: "pub"}},
		},
		nextCursor: "cur1",
		hasMore:    true,
		query:      "test",
		searchID:   1,
	}

	mdl = updateModel(mdl, msg)

	if mdl.hubExplore.loading {
		t.Error("loading should be false after search result")
	}

	if len(mdl.hubExplore.results) != 2 {
		t.Errorf("results len = %d, want 2", len(mdl.hubExplore.results))
	}

	if mdl.hubExplore.results[0].Slug != "bundle-a" {
		t.Errorf("results[0].Slug = %q, want bundle-a", mdl.hubExplore.results[0].Slug)
	}

	if !mdl.hubExplore.hasMore {
		t.Error("hasMore should be true")
	}

	if mdl.hubExplore.nextCursor != "cur1" {
		t.Errorf("nextCursor = %q, want cur1", mdl.hubExplore.nextCursor)
	}
}

func TestHubSearchResultAppend(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.results = []client.HubBundleSummary{
		{Slug: "existing"},
	}
	mdl.hubExplore.searchID = 2

	msg := hubSearchResultMsg{
		results:    []client.HubBundleSummary{{Slug: "new-one"}},
		appendMore: true,
		query:      "",
		searchID:   2,
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.hubExplore.results) != 2 {
		t.Errorf("results len = %d, want 2 (append)", len(mdl.hubExplore.results))
	}
}

func TestHubSearchErrorSetsErrorMsg(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.loading = true
	mdl.hubExplore.searchID = 1

	msg := hubSearchErrorMsg{
		err:      fmt.Errorf("network timeout"),
		query:    "test",
		searchID: 1,
	}

	mdl = updateModel(mdl, msg)

	if mdl.hubExplore.loading {
		t.Error("loading should be false after error")
	}

	if mdl.hubExplore.errorMsg != "network timeout" {
		t.Errorf("errorMsg = %q, want 'network timeout'", mdl.hubExplore.errorMsg)
	}
}

func TestHubDetailLoaded(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubDetail
	mdl.hubDetail.loading = true

	msg := hubDetailLoadedMsg{
		detail: &client.HubBundleDetail{
			HubBundleSummary: client.HubBundleSummary{
				Slug:        "my-bundle",
				DisplayName: "My Bundle",
			},
			Description: "A great bundle",
		},
	}

	mdl = updateModel(mdl, msg)

	if mdl.hubDetail.loading {
		t.Error("loading should be false after detail loaded")
	}

	if mdl.hubDetail.detail == nil {
		t.Fatal("detail should not be nil")
	}

	if mdl.hubDetail.detail.Slug != "my-bundle" {
		t.Errorf("detail.Slug = %q, want my-bundle", mdl.hubDetail.detail.Slug)
	}
}

func TestHubDetailError(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubDetail
	mdl.hubDetail.loading = true

	msg := hubDetailErrorMsg{
		err:       fmt.Errorf("not found"),
		publisher: "acme",
		slug:      "missing",
	}

	mdl = updateModel(mdl, msg)

	if mdl.hubDetail.loading {
		t.Error("loading should be false after error")
	}

	if mdl.hubDetail.errorMsg != "not found" {
		t.Errorf("errorMsg = %q, want 'not found'", mdl.hubDetail.errorMsg)
	}
}

func TestHubCategoriesLoaded(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore

	msg := hubCategoriesLoadedMsg{
		categories: []client.HubCategory{
			{Slug: "agents", DisplayName: "Agents"},
			{Slug: "tools", DisplayName: "Tools"},
		},
	}

	mdl = updateModel(mdl, msg)

	if len(mdl.hubExplore.categories) != 2 {
		t.Errorf("categories len = %d, want 2", len(mdl.hubExplore.categories))
	}
}

func TestHubDebounceValidTick(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.debounceID = 5
	mdl.hubExplore.query = "old"

	msg := hubDebounceTickMsg{id: 5, query: "new"}

	result, cmd := mdl.Update(msg)
	mdl = result.(*model)

	if !mdl.hubExplore.loading {
		t.Error("loading should be true after valid debounce tick")
	}

	if mdl.hubExplore.query != "new" {
		t.Errorf("query = %q, want 'new'", mdl.hubExplore.query)
	}

	if cmd == nil {
		t.Error("expected search command from debounce tick")
	}
}

func TestHubDebounceStaleTick(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.debounceID = 5
	mdl.hubExplore.query = "old"

	// Stale tick (id doesn't match).
	msg := hubDebounceTickMsg{id: 3, query: "stale"}

	mdl = updateModel(mdl, msg)

	if mdl.hubExplore.loading {
		t.Error("loading should remain false for stale tick")
	}

	if mdl.hubExplore.query != "old" {
		t.Errorf("query = %q, want 'old' (unchanged)", mdl.hubExplore.query)
	}
}

func TestHubDebounceSameQuery(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.debounceID = 5
	mdl.hubExplore.query = "same"

	// Tick with same query.
	msg := hubDebounceTickMsg{id: 5, query: "same"}

	result, cmd := mdl.Update(msg)
	mdl = result.(*model)

	if mdl.hubExplore.loading {
		t.Error("loading should remain false for same-query tick")
	}

	if cmd != nil {
		t.Error("expected no command for same-query tick")
	}
}

func TestHubListNavigationDown(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.focusArea = 2 // list focused
	mdl.hubExplore.results = []client.HubBundleSummary{
		{Slug: "a"},
		{Slug: "b"},
		{Slug: "c"},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.hubExplore.resultCur != 1 {
		t.Errorf("resultCur = %d, want 1 after down", mdl.hubExplore.resultCur)
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.hubExplore.resultCur != 2 {
		t.Errorf("resultCur = %d, want 2 after second down", mdl.hubExplore.resultCur)
	}

	// Clamp at end.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.hubExplore.resultCur != 2 {
		t.Errorf("resultCur = %d, want 2 (clamped)", mdl.hubExplore.resultCur)
	}
}

func TestHubListNavigationUp(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.focusArea = 2 // list focused
	mdl.hubExplore.results = []client.HubBundleSummary{{Slug: "a"}, {Slug: "b"}}
	mdl.hubExplore.resultCur = 1

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.hubExplore.resultCur != 0 {
		t.Errorf("resultCur = %d, want 0 after up", mdl.hubExplore.resultCur)
	}

	// Clamp at start.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.hubExplore.resultCur != 0 {
		t.Errorf("resultCur = %d, want 0 (clamped)", mdl.hubExplore.resultCur)
	}
}

func TestHubEnterViewsDetail(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.focusArea = 2 // list focused
	mdl.hubExplore.results = []client.HubBundleSummary{
		{Slug: "test-bundle", Publisher: client.HubPublisher{Handle: "acme"}},
	}

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEnter})

	if mdl.activeScreen != screenHubDetail {
		t.Errorf("activeScreen = %d, want screenHubDetail", mdl.activeScreen)
	}

	if mdl.hubDetail.slug != "test-bundle" {
		t.Errorf("hubDetail.slug = %q, want test-bundle", mdl.hubDetail.slug)
	}

	if mdl.hubDetail.publisher != "acme" {
		t.Errorf("hubDetail.publisher = %q, want acme", mdl.hubDetail.publisher)
	}
}

func TestHubDetailEscGoesBack(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.pushScreen(screenHubDetail)

	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyEscape})

	if mdl.activeScreen != screenHome {
		t.Errorf("activeScreen = %d, want screenHome after esc from detail", mdl.activeScreen)
	}
}

func TestHubInstallWithoutClientShowsError(t *testing.T) {
	t.Parallel()

	mdl := testModel() // nil deps
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.focusArea = 2 // list focused
	mdl.hubExplore.results = []client.HubBundleSummary{
		{Slug: "test-bundle", LatestVersion: "1.0.0"},
	}

	// Press 'i' to install.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if mdl.activeScreen != screenBundleError {
		t.Errorf("activeScreen = %d, want screenBundleError (no client)", mdl.activeScreen)
	}

	if !strings.Contains(mdl.bundleError.message, "Not authenticated") {
		t.Errorf("error message = %q, want to contain 'Not authenticated'", mdl.bundleError.message)
	}
}

func TestHubSlashFocusesSearch(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.focusArea = 2 // list focused

	// Press '/' to focus search.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if mdl.hubExplore.focusArea != 0 {
		t.Errorf("focusArea = %d, want 0 (search) after /", mdl.hubExplore.focusArea)
	}
}

func TestHubExploreView(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore

	view := mdl.View()
	if !strings.Contains(view, "Explore Hub") {
		t.Error("hub explore view should contain 'Explore Hub'")
	}

	if !strings.Contains(view, "No bundles found") {
		t.Error("hub explore view should contain empty state message")
	}
}

func TestHubExploreViewWithResults(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.results = []client.HubBundleSummary{
		{
			Slug:          "deploy-tool",
			DisplayName:   "Deploy Tool",
			Summary:       "A tool for deployment",
			BundleType:    "tool",
			LatestVersion: "1.0.0",
			StarsCount:    42,
			Publisher:     client.HubPublisher{Handle: "acme", TrustTier: "verified"},
		},
	}

	view := mdl.View()

	if !strings.Contains(view, "Deploy Tool") {
		t.Error("hub explore view should contain bundle display name")
	}

	if !strings.Contains(view, "acme/deploy-tool") {
		t.Error("hub explore view should contain publisher/slug")
	}

	if !strings.Contains(view, "v1.0.0") {
		t.Error("hub explore view should contain version")
	}
}

func TestHubDetailView(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubDetail
	mdl.hubDetail.detail = &client.HubBundleDetail{
		HubBundleSummary: client.HubBundleSummary{
			Slug:          "my-bundle",
			DisplayName:   "My Bundle",
			BundleType:    "agent",
			LatestVersion: "2.0.0",
			StarsCount:    100,
			Publisher:     client.HubPublisher{Handle: "pub", TrustTier: "verified"},
		},
		Description: "A great agent bundle",
	}

	view := mdl.View()

	if !strings.Contains(view, "My Bundle") {
		t.Error("hub detail view should contain display name")
	}

	if !strings.Contains(view, "Bundle Detail") {
		t.Error("hub detail view should contain panel title")
	}

	if !strings.Contains(view, "Install Bundle") {
		t.Error("hub detail view should contain install button")
	}

	if !strings.Contains(view, "A great agent bundle") {
		t.Error("hub detail view should contain description")
	}
}

func TestHubDetailViewLoading(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubDetail
	mdl.hubDetail.loading = true
	mdl.hubDetail.slug = "test-bundle"

	view := mdl.View()

	if !strings.Contains(view, "Loading") {
		t.Error("hub detail view should contain 'Loading' while loading")
	}
}

func TestFormatCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{15000, "15.0k"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}

	for _, test := range tests {
		got := formatCount(test.n)
		if got != test.want {
			t.Errorf("formatCount(%d) = %q, want %q", test.n, got, test.want)
		}
	}
}

func TestHubStaleSearchResultDiscarded(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.loading = true
	mdl.hubExplore.searchID = 5

	// Result from an older search (searchID=3) should be discarded.
	msg := hubSearchResultMsg{
		results:  []client.HubBundleSummary{{Slug: "stale"}},
		query:    "old",
		searchID: 3,
	}

	mdl = updateModel(mdl, msg)

	if !mdl.hubExplore.loading {
		t.Error("loading should remain true for stale search result")
	}

	if len(mdl.hubExplore.results) != 0 {
		t.Errorf("results should be empty, got %d", len(mdl.hubExplore.results))
	}
}

func TestHubStaleSearchErrorDiscarded(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubExplore
	mdl.hubExplore.loading = true
	mdl.hubExplore.searchID = 5

	// Error from an older search (searchID=2) should be discarded.
	msg := hubSearchErrorMsg{
		err:      fmt.Errorf("old error"),
		query:    "old",
		searchID: 2,
	}

	mdl = updateModel(mdl, msg)

	if !mdl.hubExplore.loading {
		t.Error("loading should remain true for stale search error")
	}

	if mdl.hubExplore.errorMsg != "" {
		t.Errorf("errorMsg should be empty, got %q", mdl.hubExplore.errorMsg)
	}
}

func TestHubDetailScrolling(t *testing.T) {
	t.Parallel()

	mdl := testModel()
	mdl.activeScreen = screenHubDetail
	mdl.hubDetail.detail = &client.HubBundleDetail{
		HubBundleSummary: client.HubBundleSummary{Slug: "test"},
	}

	// Scroll down.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyDown})

	if mdl.hubDetail.scrollOffset != 1 {
		t.Errorf("scrollOffset = %d, want 1 after down", mdl.hubDetail.scrollOffset)
	}

	// Scroll up.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.hubDetail.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 after up", mdl.hubDetail.scrollOffset)
	}

	// Don't go below 0.
	mdl = updateModel(mdl, tea.KeyMsg{Type: tea.KeyUp})

	if mdl.hubDetail.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0 (clamped)", mdl.hubDetail.scrollOffset)
	}
}
