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

	apiClient, err := newAPIClientWithKey(apiKey)
	if err != nil {
		return "", nil, err
	}

	return source, apiClient, nil
}

func newAPIClientWithKey(apiKey string) (*client.Client, error) {
	cfg := config.Load()

	httpClient, err := client.NewInstrumentedHTTPClient(cfg.CACertFile())
	if err != nil {
		return nil, clierrors.ConfigFailed("initialize HTTP client", err).
			WithHint("Set MUSH_NETWORK_CA_CERT_FILE to a readable PEM bundle, or unset it and retry")
	}

	return client.NewWithHTTPClient(cfg.APIURL(), apiKey, httpClient), nil
}

var tryAPIClient = newTryAPIClient

// newTryAPIClient returns an API client, falling back to an anonymous (no-auth)
// client when no credentials are found. The returned workspaceKeyOverride is
// "public" for anonymous clients, or empty when authenticated.
func newTryAPIClient() (auth.CredentialSource, *client.Client, string, error) {
	source, apiKey := auth.GetCredentials()

	apiClient, err := newAPIClientWithKey(apiKey)
	if err != nil {
		return auth.SourceNone, nil, "", err
	}

	if apiKey == "" {
		return auth.SourceNone, apiClient, "public", nil
	}

	return source, apiClient, "", nil
}
