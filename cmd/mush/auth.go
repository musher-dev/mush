package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long:  `Authenticate with the Musher platform using your API key.`,
	}

	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthStatusCmd())
	cmd.AddCommand(newAuthLogoutCmd())

	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var apiKeyFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with your API key",
		Long: `Authenticate with the Musher platform.

Your API key will be stored securely in your system's keyring
(macOS Keychain, Windows Credential Manager, or Linux Secret Service).

You can also set the MUSHER_API_KEY environment variable.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			prompter := prompt.New(out)

			// Check if already authenticated via env var
			if key := os.Getenv("MUSHER_API_KEY"); key != "" {
				out.Info("MUSHER_API_KEY environment variable is set")
				out.Muted("Environment variable takes precedence over stored credentials")
				out.Println()
			}

			var apiKey string
			if apiKeyFlag != "" {
				apiKey = apiKeyFlag
			} else {
				// Interactive flow: prompt for API key
				if !prompter.CanPrompt() {
					return clierrors.CannotPrompt("MUSHER_API_KEY")
				}

				var err error

				apiKey, err = prompter.Password("Enter your Musher API key")
				if err != nil {
					return fmt.Errorf("read api key prompt: %w", err)
				}
			}

			if apiKey == "" {
				return clierrors.APIKeyEmpty()
			}

			// Validate with spinner
			spin := out.Spinner("Validating API key")
			spin.Start()

			cfg := config.Load()
			apiClient := client.New(cfg.APIURL(), apiKey)

			identity, err := apiClient.ValidateKey(cmd.Context())
			if err != nil {
				spin.StopWithFailure("Invalid API key")
				return clierrors.AuthFailed(err)
			}

			spin.Stop()

			// Store in keyring
			if err := auth.StoreAPIKey(apiKey); err != nil {
				return clierrors.ConfigFailed("store credentials", err)
			}

			out.Success("Authenticated as %s (Workspace: %s)", identity.CredentialName, identity.WorkspaceName)

			return nil
		},
	}

	cmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "API key for non-interactive login (prefer MUSHER_API_KEY env var to avoid shell history exposure)")

	return cmd
}

// AuthStatus represents authentication status for JSON output.
type AuthStatus struct {
	Source     string `json:"source"`
	Credential string `json:"credential"`
	Workspace  string `json:"workspace"`
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			source, apiClient, err := newAPIClient()
			if err != nil {
				return err
			}

			// Validate with spinner
			spin := out.Spinner("Checking credentials")
			spin.Start()

			identity, err := apiClient.ValidateKey(cmd.Context())
			if err != nil {
				spin.StopWithFailure("Credentials invalid")
				return clierrors.CredentialsInvalid(err)
			}

			spin.StopWithSuccess("Authenticated")

			if out.JSON {
				if err := out.PrintJSON(AuthStatus{
					Source:     string(source),
					Credential: identity.CredentialName,
					Workspace:  identity.WorkspaceName,
				}); err != nil {
					return fmt.Errorf("print auth status json: %w", err)
				}

				return nil
			}

			out.Print("Source:     %s\n", source)
			out.Print("Credential: %s\n", identity.CredentialName)
			out.Print("Workspace:  %s\n", identity.WorkspaceName)

			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored credentials",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			if err := auth.DeleteAPIKey(); err != nil {
				// If key doesn't exist, that's fine
				if strings.Contains(err.Error(), "not found") {
					out.Muted("No stored credentials found")
					return nil
				}

				return clierrors.ConfigFailed("clear credentials", err)
			}

			out.Success("Logged out successfully")

			if os.Getenv("MUSHER_API_KEY") != "" {
				out.Println()
				out.Warning("MUSHER_API_KEY environment variable is still set")
			}

			return nil
		},
	}
}
