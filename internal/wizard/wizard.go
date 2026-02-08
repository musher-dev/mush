// Package wizard provides the initialization wizard for Mush CLI.
//
// The wizard guides users through first-time setup:
//  1. Welcome message
//  2. API key input and validation
//  3. Habitat selection
//  4. Credential storage
//  5. Next steps guidance
package wizard

import (
	"context"
	"fmt"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
)

// Wizard handles the initialization flow.
type Wizard struct {
	out      *output.Writer
	prompter *prompt.Prompter
	force    bool
}

// New creates a new initialization wizard.
func New(out *output.Writer, force bool) *Wizard {
	return &Wizard{
		out:      out,
		prompter: prompt.New(out),
		force:    force,
	}
}

// Run executes the initialization wizard.
func (w *Wizard) Run(ctx context.Context) error {
	// Welcome
	w.out.Println("Welcome to Mush!")
	w.out.Println("=================")
	w.out.Println()
	w.out.Println("Mush connects your local machine to the Musher job queue,")
	w.out.Println("executing handlers using Claude Code.")
	w.out.Println()

	// Check for existing credentials
	source, existingKey := auth.GetCredentials()
	if existingKey != "" && !w.force {
		w.out.Warning("Existing credentials found (via %s)", source)

		if !w.prompter.CanPrompt() {
			w.out.Println()
			w.out.Info("Run with --force to overwrite existing credentials")
			return nil
		}

		overwrite, err := w.prompter.Confirm("Overwrite existing credentials?", false)
		if err != nil {
			return err
		}
		if !overwrite {
			w.out.Println()
			w.out.Success("Keeping existing credentials")
			w.showNextSteps()
			return nil
		}
		w.out.Println()
	}

	// Check for non-interactive mode
	if !w.prompter.CanPrompt() {
		w.out.Failure("Cannot run init wizard in non-interactive mode")
		w.out.Println()
		w.out.Info("Either:")
		w.out.Print("  1. Run without --no-input flag\n")
		w.out.Print("  2. Set MUSHER_API_KEY environment variable\n")
		w.out.Print("  3. Run 'mush auth login' interactively\n")
		return nil
	}

	// Get API key
	w.out.Println("Step 1: Authentication")
	w.out.Println("----------------------")
	w.out.Println("Enter your Musher API key.")
	w.out.Muted("Get your API key from the Musher Console.")
	w.out.Println()

	apiKey, err := w.prompter.Password("API Key")
	if err != nil {
		return fmt.Errorf("failed to read API key: %w", err)
	}

	if apiKey == "" {
		w.out.Failure("API key cannot be empty")
		return nil
	}

	// Validate with spinner
	w.out.Println()
	spin := w.out.Spinner("Validating API key")
	spin.Start()

	cfg := config.Load()
	c := client.New(apiKey).WithBaseURL(cfg.APIURL())

	identity, err := c.ValidateKey(ctx)
	if err != nil {
		spin.StopWithFailure("Invalid API key")
		w.out.Muted("%s", err.Error())
		return nil
	}

	spin.StopWithSuccess("Authenticated")
	w.out.Print("User:      %s\n", identity.Email)
	w.out.Print("Workspace: %s\n", identity.Workspace)

	// Store credentials before habitat selection (so they persist even if user cancels)
	w.out.Println()
	spin = w.out.Spinner("Storing credentials")
	spin.Start()

	if storeErr := auth.StoreAPIKey(apiKey); storeErr != nil {
		spin.StopWithFailure("Failed to store credentials")
		w.out.Muted("%s", storeErr.Error())
		return nil
	}

	spin.StopWithSuccess("Credentials stored securely")

	// Step 2: Habitat selection
	w.out.Println()
	w.out.Println("Step 2: Select Habitat")
	w.out.Println("----------------------")
	w.out.Println("Select a habitat to link to. Habitats are execution contexts")
	w.out.Println("where agents connect and jobs are routed.")
	w.out.Println()

	// Fetch habitats
	spin = w.out.Spinner("Fetching habitats")
	spin.Start()

	habitats, err := c.ListHabitats(ctx)
	if err != nil {
		spin.StopWithFailure("Failed to fetch habitats")
		w.out.Muted("%s", err.Error())
		w.out.Println()
		w.out.Warning("You can select a habitat later with 'mush habitat select'")
		w.showNextSteps()
		return nil
	}

	spin.StopWithSuccess("Found habitats")

	if len(habitats) == 0 {
		w.out.Println()
		w.out.Warning("No habitats found in your workspace")
		w.out.Info("Create a habitat in the console first, then run 'mush habitat select'")
		w.showNextSteps()
		return nil
	}

	// Select habitat
	selected, err := prompt.SelectHabitat(habitats, w.out)
	if err != nil {
		return fmt.Errorf("failed to select habitat: %w", err)
	}

	// Save habitat to config
	if err := cfg.Set("habitat.id", selected.ID); err != nil {
		w.out.Warning("Failed to save habitat to config: %s", err.Error())
	} else {
		if err := cfg.Set("habitat.slug", selected.Slug); err != nil {
			w.out.Warning("Failed to save habitat slug to config: %s", err.Error())
		}
		w.out.Success("Selected habitat: %s (%s)", selected.Name, selected.Slug)
	}

	// Success
	w.out.Println()
	w.out.Success("Mush is ready!")
	w.showNextSteps()

	return nil
}

func (w *Wizard) showNextSteps() {
	w.out.Println()
	w.out.Println("Next steps:")
	w.out.Println("  mush doctor        Check your setup")
	w.out.Println("  mush link          Start processing jobs")
	w.out.Println("  mush --help        See all commands")
}
