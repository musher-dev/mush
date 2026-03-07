package nav

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

// --- Hub message types ---

// hubSearchResultMsg carries a successful hub search result.
type hubSearchResultMsg struct {
	results    []client.HubBundleSummary
	nextCursor string
	hasMore    bool
	appendMore bool
	query      string
	searchID   int // monotonic ID to discard out-of-order results
}

// hubSearchErrorMsg carries a hub search error.
type hubSearchErrorMsg struct {
	err      error
	query    string
	searchID int // monotonic ID to discard out-of-order results
}

// hubDetailLoadedMsg carries a loaded hub bundle detail.
type hubDetailLoadedMsg struct {
	detail *client.HubBundleDetail
}

// hubDetailErrorMsg carries a hub bundle detail error.
type hubDetailErrorMsg struct {
	err       error
	publisher string
	slug      string
}

// hubDebounceTickMsg is sent after the debounce delay to trigger a search.
type hubDebounceTickMsg struct {
	id    int
	query string
}

// --- Hub constants ---

const (
	hubSearchLimit  = 20
	hubDebounceMs   = 300
	hubDebounceWait = hubDebounceMs * time.Millisecond
)

// --- Hub commands ---

// cmdSearchHub searches for bundles in the hub.
func cmdSearchHub(ctx context.Context, baseURL, query, bundleType, sort string, limit int, cursor string, appendMore bool, searchID int) tea.Cmd {
	return func() tea.Msg {
		c := client.New(baseURL, "")

		resp, err := c.SearchHubBundles(navBaseCtx(ctx), query, bundleType, sort, limit, cursor)
		if err != nil {
			return hubSearchErrorMsg{err: err, query: query, searchID: searchID}
		}

		return hubSearchResultMsg{
			results:    resp.Data,
			nextCursor: resp.Meta.NextCursor,
			hasMore:    resp.Meta.HasMore,
			appendMore: appendMore,
			query:      query,
			searchID:   searchID,
		}
	}
}

// cmdGetHubDetail fetches full details for a hub bundle.
func cmdGetHubDetail(ctx context.Context, baseURL, publisher, slug string) tea.Cmd {
	return func() tea.Msg {
		c := client.New(baseURL, "")

		detail, err := c.GetHubBundleDetail(navBaseCtx(ctx), publisher, slug)
		if err != nil {
			return hubDetailErrorMsg{err: err, publisher: publisher, slug: slug}
		}

		return hubDetailLoadedMsg{detail: detail}
	}
}

// cmdHubDebounceTick schedules a debounce tick for search.
func cmdHubDebounceTick(id int, query string) tea.Cmd {
	return tea.Tick(hubDebounceWait, func(_ time.Time) tea.Msg {
		return hubDebounceTickMsg{id: id, query: query}
	})
}
