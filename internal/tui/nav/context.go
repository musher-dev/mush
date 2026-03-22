package nav

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/config"
)

// contextInfoMsg carries async-loaded context data back to the model.
type contextInfoMsg struct {
	authStatus       string
	organizationName string
	organizationID   string
	credentialName   string
	userFullName     string
	username         string
	greeting         string
}

var nowFunc = time.Now

// cmdLoadContext loads auth status and identity context asynchronously.
func cmdLoadContext(ctx context.Context, deps *Dependencies) tea.Cmd {
	return func() tea.Msg {
		msg := contextInfoMsg{
			authStatus: "not authenticated",
			greeting:   greetingForTime(nowFunc()),
		}

		if deps == nil {
			return msg
		}

		// 1. Check auth credentials (local, fast).
		var apiURL string
		if deps.Config != nil {
			apiURL = deps.Config.APIURL()
		} else {
			apiURL = config.Load().APIURL()
		}

		source, apiKey := auth.GetCredentials(apiURL)
		if source != auth.SourceNone && apiKey != "" {
			msg.authStatus = "authenticated"
		}

		// 2. If authed and client available, validate key to get organization info.
		if msg.authStatus == "authenticated" && deps.Client != nil {
			identity, err := deps.Client.ValidateKey(navBaseCtx(ctx))
			if err == nil {
				msg.organizationName = identity.OrganizationName
				msg.organizationID = identity.OrganizationID
				msg.credentialName = identity.CredentialName
			}

			profile, err := deps.Client.GetCurrentUserProfile(navBaseCtx(ctx))
			if err == nil {
				msg.userFullName = profile.FullName
				msg.username = profile.Username
			}
		}

		return msg
	}
}

func greetingForTime(now time.Time) string {
	hour := now.Hour()

	switch {
	case hour < 12:
		return "Good morning"
	case hour < 18:
		return "Good afternoon"
	default:
		return "Good evening"
	}
}
