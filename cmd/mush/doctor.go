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
		Example: `  mush doctor`,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := output.FromContext(cmd.Context())

			out.Println("Mush Doctor")
			out.Println("============")
			out.Println()

			// Run diagnostics
			runner := doctor.New()
			results := runner.Run(cmd.Context())

			// Display results
			doctor.RenderResults(results, out.Print, out.Success, out.Warning, out.Failure, out.Muted)

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
