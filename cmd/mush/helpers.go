package main

import (
	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
)

var apiClientFactory = newAPIClient

// newAPIClient creates an authenticated API client using stored credentials
// and the configured API URL. Returns a CLIError if not authenticated.
//
// This consolidates the repeated pattern of:
//
//	source, apiKey := auth.GetCredentials()
//	cfg := config.Load()
//	c := client.New(cfg.APIURL(), apiKey)
func newAPIClient() (auth.CredentialSource, *client.Client, error) {
	source, apiKey := auth.GetCredentials()
	if apiKey == "" {
		return "", nil, clierrors.NotAuthenticated()
	}

	cfg := config.Load()
	c := client.New(cfg.APIURL(), apiKey)

	return source, c, nil
}

var tryAPIClient = newTryAPIClient

// newTryAPIClient returns an API client, falling back to an anonymous (no-auth)
// client when no credentials are found. The returned workspaceKeyOverride is
// "public" for anonymous clients, or empty when authenticated.
func newTryAPIClient() (auth.CredentialSource, *client.Client, string, error) {
	source, apiKey := auth.GetCredentials()
	cfg := config.Load()
	c := client.New(cfg.APIURL(), apiKey)

	if apiKey == "" {
		return auth.SourceNone, c, "public", nil
	}

	return source, c, "", nil
}
