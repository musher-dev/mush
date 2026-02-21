// Package doctor provides diagnostic checks for Mush CLI health.
//
// This package implements a check framework that validates:
//   - API connectivity and response time
//   - Authentication status and credential source
//   - Claude CLI availability and version
//   - CLI version against latest release
package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/musher-dev/mush/internal/auth"
	"github.com/musher-dev/mush/internal/buildinfo"
	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/config"
	"github.com/musher-dev/mush/internal/harness"
	"github.com/musher-dev/mush/internal/update"
)

// Status represents the result of a diagnostic check.
type Status int

const (
	// StatusPass indicates the check passed.
	StatusPass Status = iota
	// StatusWarn indicates a non-critical issue.
	StatusWarn
	// StatusFail indicates a critical failure.
	StatusFail
)

// Result holds the outcome of a single check.
type Result struct {
	Name    string
	Status  Status
	Message string
	Detail  string // Optional additional detail
}

// Check is a diagnostic check function.
type Check func(ctx context.Context) Result

// Runner executes diagnostic checks.
type Runner struct {
	checks []namedCheck
}

type namedCheck struct {
	name  string
	check Check
}

// New creates a new diagnostic runner.
func New() *Runner {
	r := &Runner{}

	// Register default checks
	r.AddCheck("API Connectivity", checkAPIConnectivity)
	r.AddCheck("Authentication", checkAuthentication)
	r.AddCheck("Claude CLI", checkClaudeCLI)
	r.AddCheck("CLI Version", checkCLIVersion)

	return r
}

// AddCheck registers a diagnostic check.
func (r *Runner) AddCheck(name string, check Check) {
	r.checks = append(r.checks, namedCheck{name: name, check: check})
}

// Run executes all registered checks and returns the results.
func (r *Runner) Run(ctx context.Context) []Result {
	results := make([]Result, 0, len(r.checks))

	for _, nc := range r.checks {
		result := nc.check(ctx)
		result.Name = nc.name
		results = append(results, result)
	}

	return results
}

// Summary returns counts of passed, failed, and warning checks.
func Summary(results []Result) (passed, failed, warnings int) {
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			passed++
		case StatusFail:
			failed++
		case StatusWarn:
			warnings++
		}
	}

	return passed, failed, warnings
}

// checkAPIConnectivity tests connection to the API endpoint.
func checkAPIConnectivity(ctx context.Context) Result {
	cfg := config.Load()
	apiURL := cfg.APIURL()

	start := time.Now()

	// Create a simple HTTP client to test connectivity
	c := client.New(apiURL, "test-key")

	// We expect this to fail auth, but succeed at connecting
	_, err := c.ValidateKey(ctx)
	elapsed := time.Since(start)

	// If we get an auth error, that means connectivity works
	if err != nil && strings.Contains(err.Error(), "invalid") {
		return Result{
			Status:  StatusPass,
			Message: fmt.Sprintf("%s (%dms)", apiURL, elapsed.Milliseconds()),
		}
	}

	// If we get a connection error, that's a failure
	if err != nil {
		return Result{
			Status:  StatusFail,
			Message: apiURL,
			Detail:  err.Error(),
		}
	}

	// Connection succeeded (shouldn't happen with test key, but handle it)
	return Result{
		Status:  StatusPass,
		Message: fmt.Sprintf("%s (%dms)", apiURL, elapsed.Milliseconds()),
	}
}

// checkAuthentication validates stored credentials.
func checkAuthentication(ctx context.Context) Result {
	source, apiKey := auth.GetCredentials()

	if apiKey == "" {
		return Result{
			Status:  StatusFail,
			Message: "Not authenticated",
			Detail:  "Run 'mush auth login' to authenticate",
		}
	}

	// Validate the key
	cfg := config.Load()
	c := client.New(cfg.APIURL(), apiKey)

	identity, err := c.ValidateKey(ctx)
	if err != nil {
		return Result{
			Status:  StatusFail,
			Message: fmt.Sprintf("Invalid credentials (via %s)", source),
			Detail:  err.Error(),
		}
	}

	return Result{
		Status:  StatusPass,
		Message: fmt.Sprintf("%s (via %s)", identity.CredentialName, source),
	}
}

// checkClaudeCLI verifies Claude Code CLI is available.
func checkClaudeCLI(ctx context.Context) Result {
	if !harness.AvailableFunc("claude")() {
		return Result{
			Status:  StatusFail,
			Message: "Not found in PATH",
			Detail:  "Install from https://claude.ai/download",
		}
	}

	// Try to get version
	cmd := exec.CommandContext(ctx, "claude", "--version")

	out, err := cmd.Output()
	if err != nil {
		return Result{
			Status:  StatusWarn,
			Message: "Found but version unknown",
		}
	}

	version := strings.TrimSpace(string(out))
	// Extract just the version number if there's extra output
	if idx := strings.Index(version, "\n"); idx > 0 {
		version = version[:idx]
	}

	// Get the path (we already know it exists from the check above)
	path, err := exec.LookPath("claude")
	if err != nil {
		path = "(unknown path)"
	}

	return Result{
		Status:  StatusPass,
		Message: fmt.Sprintf("%s at %s", version, path),
	}
}

// checkCLIVersion checks the CLI version against the latest release.
func checkCLIVersion(ctx context.Context) Result {
	current := buildinfo.Version

	if current == "dev" {
		return Result{
			Status:  StatusWarn,
			Message: "Development build (version check skipped)",
		}
	}

	if update.IsDisabled() {
		return Result{
			Status:  StatusPass,
			Message: fmt.Sprintf("v%s (update checks disabled)", current),
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	updater, err := update.NewUpdater()
	if err != nil {
		return Result{
			Status:  StatusWarn,
			Message: fmt.Sprintf("v%s (could not check for updates)", current),
			Detail:  err.Error(),
		}
	}

	info, err := updater.CheckLatest(checkCtx, current)
	if err != nil {
		return Result{
			Status:  StatusWarn,
			Message: fmt.Sprintf("v%s (could not check for updates)", current),
			Detail:  err.Error(),
		}
	}

	if info.UpdateAvailable {
		return Result{
			Status:  StatusWarn,
			Message: fmt.Sprintf("v%s (v%s available)", current, info.LatestVersion),
			Detail:  "Run 'mush update' to update",
		}
	}

	return Result{
		Status:  StatusPass,
		Message: fmt.Sprintf("v%s (latest)", current),
	}
}

// RenderResults formats diagnostic results to the given output writer.
func RenderResults(results []Result, printFn, successFn, warningFn, failureFn, mutedFn func(format string, args ...any)) {
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
		case StatusPass:
			successFn("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
		case StatusWarn:
			warningFn("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
		case StatusFail:
			failureFn("%-*s%s", len(r.Name)+padding, r.Name, r.Message)
		default:
			printFn("%s %-*s%s\n", symbol, len(r.Name)+padding, r.Name, r.Message)
		}

		if r.Detail != "" {
			mutedFn("    %s", r.Detail)
		}
	}
}

// Symbol returns the status symbol for display.
func (s Status) Symbol() string {
	switch s {
	case StatusPass:
		return checkMark
	case StatusWarn:
		return warningMark
	case StatusFail:
		return xMark
	default:
		return "?"
	}
}

const (
	checkMark   = "\u2713" // ✓
	xMark       = "\u2717" // ✗
	warningMark = "\u26A0" // ⚠
)
