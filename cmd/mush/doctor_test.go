package main

import (
	"bytes"
	"testing"

	"github.com/musher-dev/mush/internal/doctor"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
	"github.com/musher-dev/mush/internal/testutil"
)

// renderDoctorOutput reproduces the doctor command's output formatting logic
// with the given results, so golden tests can run without real checks.
func renderDoctorOutput(results []doctor.Result) string {
	var buf bytes.Buffer

	term := &terminal.Info{IsTTY: false, NoColor: true, Width: 80, Height: 24}
	out := output.NewWriter(&buf, &buf, term)

	out.Println("Mush Doctor")
	out.Println("============")
	out.Println()

	doctor.RenderResults(results, out.Print, out.Success, out.Warning, out.Failure, out.Muted)

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

	return buf.String()
}

func TestDoctorOutput_AllPass_Golden(t *testing.T) {
	results := []doctor.Result{
		{Name: "API Connectivity", Status: doctor.StatusPass, Message: "https://api.musher.dev (42ms)"},
		{Name: "Authentication", Status: doctor.StatusPass, Message: "sa-test (via keyring)"},
		{Name: "Claude CLI", Status: doctor.StatusPass, Message: "1.0.20 at /usr/local/bin/claude"},
		{Name: "CLI Version", Status: doctor.StatusPass, Message: "v2.3.0 (latest)"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_all_pass.golden")
}

func TestDoctorOutput_Mixed_Golden(t *testing.T) {
	results := []doctor.Result{
		{Name: "API Connectivity", Status: doctor.StatusPass, Message: "https://api.musher.dev (42ms)"},
		{Name: "Authentication", Status: doctor.StatusFail, Message: "Not authenticated", Detail: "Run 'mush auth login' to authenticate"},
		{Name: "Claude CLI", Status: doctor.StatusWarn, Message: "Found but version unknown"},
		{Name: "CLI Version", Status: doctor.StatusWarn, Message: "v2.2.0 (v2.3.0 available)", Detail: "Run 'mush update' to update"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_mixed.golden")
}

func TestDoctorOutput_AllFail_Golden(t *testing.T) {
	results := []doctor.Result{
		{Name: "API Connectivity", Status: doctor.StatusFail, Message: "https://api.musher.dev", Detail: "connection refused"},
		{Name: "Authentication", Status: doctor.StatusFail, Message: "Not authenticated", Detail: "Run 'mush auth login' to authenticate"},
		{Name: "Claude CLI", Status: doctor.StatusFail, Message: "Not found in PATH", Detail: "Install from https://claude.ai/download"},
		{Name: "CLI Version", Status: doctor.StatusWarn, Message: "Development build (version check skipped)"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_all_fail.golden")
}
