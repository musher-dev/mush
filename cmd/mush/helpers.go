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
// Config is loaded first to determine the API URL, then credentials are
// resolved for that host.
func newAPIClient() (auth.CredentialSource, *client.Client, error) {
	cfg := config.Load()

	source, apiKey := auth.GetCredentials(cfg.APIURL())
	if apiKey == "" {
		return "", nil, clierrors.NotAuthenticated()
	}

	apiClient, err := newAPIClientFromConfig(cfg, apiKey)
	if err != nil {
		return "", nil, err
	}

	return source, apiClient, nil
}

func newAPIClientWithKey(apiKey string) (*client.Client, error) {
	cfg := config.Load()

	return newAPIClientFromConfig(cfg, apiKey)
}

func newAPIClientFromConfig(cfg *config.Config, apiKey string) (*client.Client, error) {
	httpClient, err := client.NewInstrumentedHTTPClient(cfg.CACertFile())
	if err != nil {
		return nil, clierrors.ConfigFailed("initialize HTTP client", err).
			WithHint("Set MUSHER_NETWORK_CA_CERT_FILE to a readable PEM bundle, or unset it and retry")
	}

	return client.NewWithHTTPClient(cfg.APIURL(), apiKey, httpClient), nil
}

var tryAPIClient = newTryAPIClient

// newTryAPIClient returns an API client, falling back to an anonymous (no-auth)
// client when no credentials are found. The returned workspaceKeyOverride is
// "public" for anonymous clients, or empty when authenticated.
func newTryAPIClient() (auth.CredentialSource, *client.Client, string, error) {
	cfg := config.Load()

	source, apiKey := auth.GetCredentials(cfg.APIURL())

	apiClient, err := newAPIClientFromConfig(cfg, apiKey)
	if err != nil {
		return auth.SourceNone, nil, "", err
	}

	if apiKey == "" {
		return auth.SourceNone, apiClient, "public", nil
	}

	return source, apiClient, "", nil
}
