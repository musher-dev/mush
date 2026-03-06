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
		{Name: "Directory Structure", Status: doctor.StatusPass, Message: "Config, state, and cache directories OK"},
		{Name: "Config File", Status: doctor.StatusPass, Message: "No config file (using defaults)"},
		{Name: "Credentials File", Status: doctor.StatusPass, Message: "Not present (using keyring or env)"},
		{Name: "Proxy Environment", Status: doctor.StatusPass, Message: "No proxy environment variables detected"},
		{Name: "Custom CA Bundle", Status: doctor.StatusPass, Message: "Not configured"},
		{Name: "API Connectivity", Status: doctor.StatusPass, Message: "https://api.musher.dev (42ms)"},
		{Name: "Clock Skew", Status: doctor.StatusPass, Message: "Within tolerance (1s)"},
		{Name: "Authentication", Status: doctor.StatusPass, Message: "sa-test (via keyring)"},
		{Name: "CLI Version", Status: doctor.StatusPass, Message: "v2.3.0 (latest)"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_all_pass.golden")
}

func TestDoctorOutput_Mixed_Golden(t *testing.T) {
	results := []doctor.Result{
		{Name: "Directory Structure", Status: doctor.StatusPass, Message: "Config, state, and cache directories OK"},
		{Name: "Config File", Status: doctor.StatusPass, Message: "No config file (using defaults)"},
		{Name: "Credentials File", Status: doctor.StatusWarn, Message: "Credentials file too permissive (0644)", Detail: "chmod 600 /home/user/.config/mush/api-key"},
		{Name: "Proxy Environment", Status: doctor.StatusPass, Message: "No proxy environment variables detected"},
		{Name: "Custom CA Bundle", Status: doctor.StatusPass, Message: "Not configured"},
		{Name: "API Connectivity", Status: doctor.StatusPass, Message: "https://api.musher.dev (42ms)"},
		{Name: "Clock Skew", Status: doctor.StatusPass, Message: "Within tolerance (1s)"},
		{Name: "Authentication", Status: doctor.StatusFail, Message: "Not authenticated", Detail: "Run 'mush auth login' to authenticate"},
		{Name: "CLI Version", Status: doctor.StatusWarn, Message: "v2.2.0 (v2.3.0 available)", Detail: "Run 'mush update' to update"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_mixed.golden")
}

func TestDoctorOutput_AllFail_Golden(t *testing.T) {
	results := []doctor.Result{
		{Name: "Directory Structure", Status: doctor.StatusFail, Message: "Cannot resolve directories", Detail: "$HOME must be set"},
		{Name: "Config File", Status: doctor.StatusPass, Message: "No config file (using defaults)"},
		{Name: "Credentials File", Status: doctor.StatusPass, Message: "Not present (using keyring or env)"},
		{Name: "Proxy Environment", Status: doctor.StatusPass, Message: "No proxy environment variables detected"},
		{Name: "Custom CA Bundle", Status: doctor.StatusPass, Message: "Not configured"},
		{Name: "API Connectivity", Status: doctor.StatusFail, Message: "https://api.musher.dev", Detail: "connection refused"},
		{Name: "Clock Skew", Status: doctor.StatusWarn, Message: "Clock skew check skipped", Detail: "API not reachable"},
		{Name: "Authentication", Status: doctor.StatusFail, Message: "Not authenticated", Detail: "Run 'mush auth login' to authenticate"},
		{Name: "CLI Version", Status: doctor.StatusWarn, Message: "Development build (version check skipped)"},
	}

	got := renderDoctorOutput(results)
	testutil.AssertGolden(t, got, "doctor_all_fail.golden")
}
