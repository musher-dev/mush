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
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/transcript"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect transcript history from PTY sessions",
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
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			dir := config.Load().HistoryDir()

			sessions, err := transcript.ListSessions(dir)
			if err != nil {
				return fmt.Errorf("list transcript sessions: %w", err)
			}

			if out.JSON {
				if err := out.PrintJSON(sessions); err != nil {
					return fmt.Errorf("print transcript sessions json: %w", err)
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
		Args:  cobra.ExactArgs(1),
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
					return fmt.Errorf("read transcript events: %w", err)
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
					return fmt.Errorf("read live transcript events: %w", err)
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
	var olderThan string

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete transcript sessions older than a duration",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())
			cfg := config.Load()
			window := cfg.HistoryRetention()

			if olderThan != "" {
				d, err := time.ParseDuration(olderThan)
				if err != nil {
					return fmt.Errorf("invalid duration for --older-than: %w", err)
				}

				window = d
			}

			removed, err := transcript.PruneOlderThan(cfg.HistoryDir(), time.Now().Add(-window))
			if err != nil {
				return fmt.Errorf("prune transcript sessions: %w", err)
			}

			out.Success("Removed %d transcript session(s)", removed)

			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Override retention window (example: 168h)")

	return cmd
}
