package nav

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/transcript"
)

// contextInfoMsg carries async-loaded context data back to the model.
type contextInfoMsg struct {
	authStatus     string
	workspaceName  string
	workspaceID    string
	recentSessions []transcript.Session
}

// maxRecentSessions is the number of recent sessions shown in the context panel.
const maxRecentSessions = 3

// cmdLoadContext loads auth status, workspace name, and recent sessions asynchronously.
func cmdLoadContext(deps *Dependencies) tea.Cmd {
	return func() tea.Msg {
		msg := contextInfoMsg{
			authStatus: "not authenticated",
		}

		if deps == nil {
			return msg
		}

		// 1. Check auth credentials (local, fast).
		source, apiKey := auth.GetCredentials()
		if source != auth.SourceNone && apiKey != "" {
			msg.authStatus = "authenticated"
		}

		// 2. If authed and client available, validate key to get workspace info.
		if msg.authStatus == "authenticated" && deps.Client != nil {
			ctx := context.Background()

			identity, err := deps.Client.ValidateKey(ctx)
			if err == nil {
				msg.workspaceName = identity.WorkspaceName
				msg.workspaceID = identity.WorkspaceID
			}
		}

		// 3. Load recent transcript sessions.
		sessions, err := transcript.ListSessions("")
		if err == nil && len(sessions) > maxRecentSessions {
			sessions = sessions[:maxRecentSessions]
		}

		msg.recentSessions = sessions

		return msg
	}
}
