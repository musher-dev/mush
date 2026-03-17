package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/musher-dev/mush/internal/devhooks"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mush-hook <pre-push|commit-msg> [args...]")
	}

	switch args[0] {
	case "pre-push":
		if err := devhooks.RunPrePush(ctx, os.Stdin, runTask, gitDiff); err != nil {
			return fmt.Errorf("run pre-push checks: %w", err)
		}

		return nil
	case "commit-msg":
		if len(args) < 2 {
			return fmt.Errorf("commit-msg hook requires commit message file path")
		}

		if err := validateCommitMessageFile(args[1]); err != nil {
			return fmt.Errorf("run commit-msg checks: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unknown mush-hook subcommand %q", args[0])
	}
}

func validateCommitMessageFile(path string) error {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("open commit message file %s: %w", path, err)
	}

	defer func() {
		_ = file.Close()
	}()

	if err := devhooks.ValidateCommitMessage(file); err != nil {
		return fmt.Errorf("validate commit message: %w", err)
	}

	return nil
}

func runTask(ctx context.Context, taskName string, args ...string) error {
	cmdArgs := append([]string{taskName}, args...)
	//nolint:gosec // command is fixed to the checked-in Task binary entrypoint.
	cmd := exec.CommandContext(ctx, "task", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run task %s: %w", taskName, err)
	}

	return nil
}

func gitDiff(ctx context.Context, remoteSHA, localSHA string) ([]string, error) {
	//nolint:gosec // SHAs come from git's pre-push ref stream and are only used as git diff endpoints.
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", remoteSHA+".."+localSHA)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s..%s: %w", remoteSHA, localSHA, err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	return lines, nil
}
