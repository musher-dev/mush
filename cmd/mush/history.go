package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/ansi"
	"github.com/musher-dev/mush/internal/config"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/prompt"
	"github.com/musher-dev/mush/internal/transcript"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect transcript history from PTY sessions",
		Long: `Inspect and manage transcript history captured during PTY harness sessions.

Transcripts are stored locally and can be listed, viewed, or pruned to free
disk space.`,
	}

	cmd.AddCommand(newHistoryListCmd())
	cmd.AddCommand(newHistoryViewCmd())
	cmd.AddCommand(newHistoryPruneCmd())

	return cmd
}

func newHistoryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored transcript sessions",
		Long:  `List all locally stored transcript sessions with their start and close times.`,
		Example: `  mush history list
  mush history list --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			dir := config.Load().HistoryDir()

			sessions, err := transcript.ListSessions(dir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list transcript sessions", err).
					WithHint("Check that the history directory exists and is readable")
			}

			if out.JSON {
				if err := out.PrintJSON(map[string]any{"items": sessions}); err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to write JSON output", err)
				}

				return nil
			}

			if len(sessions) == 0 {
				out.Muted("No transcript sessions found.")
				return nil
			}

			for _, session := range sessions {
				closed := "open"
				if session.ClosedAt != nil {
					closed = session.ClosedAt.Format(time.RFC3339)
				}

				out.Print("%s  started=%s  closed=%s\n", session.SessionID, session.StartedAt.Format(time.RFC3339), closed)
			}

			return nil
		},
	}
}

func newHistoryViewCmd() *cobra.Command {
	var (
		search string
		follow bool
		raw    bool
	)

	cmd := &cobra.Command{
		Use:   "view <session-id>",
		Short: "View transcript events for a session",
		Long: `Display the captured transcript events for a specific session.

Use --follow to tail the transcript in real time while a session is active.
Use --search to filter output to lines matching a substring.`,
		Example: `  mush history view SESSION_ID
  mush history view SESSION_ID --follow`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			out := output.FromContext(cmd.Context())
			dir := config.Load().HistoryDir()

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigCh := make(chan os.Signal, 1)

			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			go func() {
				select {
				case <-sigCh:
					cancel()
				case <-ctx.Done():
				}
			}()

			var lastSeq uint64

			events, err := transcript.ReadEvents(dir, sessionID)
			if err != nil {
				if !follow {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read transcript events", err)
				}
			} else {
				for _, event := range events {
					if event.Seq <= lastSeq {
						continue
					}

					line := event.Text
					if !raw {
						line = ansi.Strip(line)
					}

					if search != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(search)) {
						lastSeq = event.Seq
						continue
					}

					out.Print("%s\n", strings.TrimRight(line, "\n"))

					lastSeq = event.Seq
				}
			}

			if !follow {
				return nil
			}

			var liveOffset int64
			for {
				liveEvents, nextOffset, err := transcript.ReadLiveEventsFrom(dir, sessionID, liveOffset)
				if err != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read live transcript events", err)
				}

				liveOffset = nextOffset

				for _, event := range liveEvents {
					if event.Seq <= lastSeq {
						continue
					}

					line := event.Text
					if !raw {
						line = ansi.Strip(line)
					}

					if search != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(search)) {
						lastSeq = event.Seq
						continue
					}

					out.Print("%s\n", strings.TrimRight(line, "\n"))

					lastSeq = event.Seq
				}

				select {
				case <-ctx.Done():
					return nil
				case <-time.After(1 * time.Second):
				}
			}
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Filter output to lines containing this substring")
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow updates as new transcript events are written")
	cmd.Flags().BoolVar(&raw, "raw", false, "Show raw output including ANSI escape sequences")

	return cmd
}

func newHistoryPruneCmd() *cobra.Command {
	var (
		olderThan string
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete transcript sessions older than a duration",
		Long: `Delete transcript sessions older than the configured retention window.

The default retention comes from the history.retention config key (default 720h).
Use --older-than to override. Requires confirmation unless --force is passed.`,
		Example: `  mush history prune
  mush history prune --older-than 168h
  mush history prune --force`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			cfg := config.Load()
			window := cfg.HistoryRetention()

			if olderThan != "" {
				parsed, err := time.ParseDuration(olderThan)
				if err != nil {
					return clierrors.Wrap(clierrors.ExitUsage, "Invalid duration for --older-than", err).
						WithHint("Use Go duration format, e.g. 168h, 24h, 30m")
				}

				window = parsed
			}

			// Preview what will be pruned.
			dir := cfg.HistoryDir()
			cutoff := time.Now().Add(-window)

			sessions, err := transcript.ListSessions(dir)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to list transcript sessions", err)
			}

			var count int

			for _, s := range sessions {
				if s.StartedAt.Before(cutoff) {
					count++
				}
			}

			if count == 0 {
				out.Muted("No transcript sessions older than %s", window)
				return nil
			}

			out.Print("Found %d session(s) older than %s\n", count, window)

			// Require confirmation.
			if !force {
				if out.NoInput {
					return clierrors.New(clierrors.ExitUsage, "Cannot confirm prune in non-interactive mode").
						WithHint("Use --force to skip confirmation")
				}

				prompter := prompt.New(out)

				confirmed, promptErr := prompter.Confirm(
					fmt.Sprintf("Delete %d transcript session(s)?", count),
					false,
				)
				if promptErr != nil {
					return clierrors.Wrap(clierrors.ExitGeneral, "Failed to read confirmation", promptErr)
				}

				if !confirmed {
					out.Info("Prune canceled")
					return nil
				}
			}

			removed, err := transcript.PruneOlderThan(dir, cutoff)
			if err != nil {
				return clierrors.Wrap(clierrors.ExitGeneral, "Failed to prune transcript sessions", err)
			}

			out.Success("Removed %d transcript session(s)", removed)

			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Override retention window (example: 168h)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
