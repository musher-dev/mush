package nav

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/client"
)

// myBundlesLoadedMsg carries the result of loading the user's published bundles.
type myBundlesLoadedMsg struct {
	bundles  []client.HubBundleSummary
	errorMsg string
}

// cmdLoadMyBundles loads the authenticated user's published bundles via their publisher handle.
func cmdLoadMyBundles(ctx context.Context, c *client.Client) tea.Cmd {
	return func() tea.Msg {
		publishers, err := c.GetRunnerPublishers(navBaseCtx(ctx))
		if err != nil {
			if errors.Is(err, client.ErrEndpointNotAvailable) {
				return myBundlesLoadedMsg{errorMsg: "Coming soon"}
			}

			return myBundlesLoadedMsg{errorMsg: "Could not load publishers"}
		}

		if len(publishers) == 0 {
			return myBundlesLoadedMsg{errorMsg: "No publisher handle found"}
		}

		resp, err := c.ListPublisherBundles(navBaseCtx(ctx), publishers[0].Handle, 20, "") //nolint:mnd // reasonable page size
		if err != nil {
			return myBundlesLoadedMsg{errorMsg: "Could not load bundles"}
		}

		return myBundlesLoadedMsg{bundles: resp.Data}
	}
}
