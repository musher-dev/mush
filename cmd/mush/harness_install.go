package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/executil"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
	"github.com/musher-dev/mush/internal/tui/nav"
)

// handleHarnessInstall runs install commands for missing harnesses after user confirmation.
func handleHarnessInstall(ctx context.Context, out *output.Writer, result *nav.Result) error {
	if len(result.InstallCommands) == 0 {
		return nil
	}

	out.Print("\nThe following commands will be run to install the harness:\n")

	for _, args := range result.InstallCommands {
		if len(args) == 0 {
			continue
		}

		out.Print("  %s\n", strings.Join(args, " "))
	}

	out.Print("\n")

	p := prompt.New(out)
	if !p.CanPrompt() {
		out.Muted("Non-interactive mode — run the commands above manually to install.")
		return nil
	}

	ok, promptErr := p.Confirm("Run these install commands?", false)
	if promptErr != nil {
		return clierrors.Wrap(clierrors.ExitGeneral, "Install confirmation failed", promptErr)
	}

	if !ok {
		out.Muted("Install skipped. Run the commands above manually to install.")
		return nil
	}

	for _, args := range result.InstallCommands {
		if len(args) == 0 {
			continue
		}

		out.Info("Installing: %s", strings.Join(args, " "))

		cmd, err := executil.CommandContext(ctx, args[0], args[1:]...)
		if err != nil {
			return &clierrors.CLIError{
				Message: fmt.Sprintf("Install failed: %s", strings.Join(args, " ")),
				Hint:    err.Error(),
				Code:    clierrors.ExitGeneral,
			}
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return &clierrors.CLIError{
				Message: fmt.Sprintf("Install failed: %s", strings.Join(args, " ")),
				Hint:    "Check your network and package manager, then try again",
				Code:    clierrors.ExitGeneral,
			}
		}

		out.Success("Installed successfully")
	}

	return nil
}
