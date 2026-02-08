package main

import (
	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
)

// newAPIClient creates an authenticated API client using stored credentials
// and the configured API URL. Returns a CLIError if not authenticated.
//
// This consolidates the repeated pattern of:
//
//	source, apiKey := auth.GetCredentials()
//	cfg := config.Load()
//	c := client.New(apiKey).WithBaseURL(cfg.APIURL())
func newAPIClient() (auth.CredentialSource, *client.Client, error) {
	source, apiKey := auth.GetCredentials()
	if apiKey == "" {
		return "", nil, clierrors.NotAuthenticated()
	}
	cfg := config.Load()
	c := client.New(apiKey).WithBaseURL(cfg.APIURL())
	return source, c, nil
}
