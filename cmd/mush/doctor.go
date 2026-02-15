package main

import (
	"github.com/spf13/cobra"

	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/output"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common issues",
		Long: `Run diagnostic checks to identify configuration and connectivity issues.

Checks performed:
  - API connectivity and response time
  - Authentication status and credential source
  - Claude CLI availability and version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			out.Println("Mush Doctor")
			out.Println("============")
			out.Println()

			// Run diagnostics
			runner := doctor.New()
			results := runner.Run(cmd.Context())

			// Display results
			maxNameLen := 0
			for _, r := range results {
				if len(r.Name) > maxNameLen {
					maxNameLen = len(r.Name)
				}
			}

			for _, r := range results {
				symbol := r.Status.Symbol()
				padding := maxNameLen - len(r.Name) + 4

				switch r.Status {
				case doctor.StatusPass:
					out.Success("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
				case doctor.StatusWarn:
					out.Warning("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
				case doctor.StatusFail:
					out.Failure("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
				default:
					out.Print("%s %-*s%s\n", symbol, len(r.Name)+padding, r.Name, r.Message)
				}

				if r.Detail != "" {
					out.Muted("    %s", r.Detail)
				}
			}

			// Summary
			passed, failed, warnings := doctor.Summary(results)
			out.Println()
			out.Print("%d passed", passed)
			if failed > 0 {
				out.Print(", %d failed", failed)
			}
			if warnings > 0 {
				out.Print(", %d warning(s)", warnings)
			}
			out.Println()

			return nil
		},
	}
}
