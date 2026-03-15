package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/musher-dev/mush/internal/harness/harnesstype"
)

// HealthStatus represents the result of a single health check.
type HealthStatus int

const (
	// HealthPass indicates the check passed.
	HealthPass HealthStatus = iota
	// HealthWarn indicates the check passed with a warning.
	HealthWarn
	// HealthFail indicates the check failed.
	HealthFail
)

// HealthResult holds the outcome of a single health check.
type HealthResult struct {
	Check   string
	Message string
	Detail  string
	Status  HealthStatus
}

// HealthReport holds the results of all health checks for a single provider.
type HealthReport struct {
	ProviderName string
	DisplayName  string
	InstallHint  string
	Results      []HealthResult
}

// CheckHealth runs all applicable health checks for the named provider.
func CheckHealth(ctx context.Context, name string) (*HealthReport, error) {
	spec, ok := GetProvider(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	report := &HealthReport{
		ProviderName: spec.Name,
		DisplayName:  spec.DisplayName,
	}

	if spec.Status != nil {
		report.InstallHint = spec.Status.InstallHint
	}

	// Binary check always runs.
	binaryResult := checkBinary(spec)
	report.Results = append(report.Results, binaryResult)

	// Remaining checks only run if the binary is available.
	if binaryResult.Status == HealthFail {
		return report, nil
	}

	if spec.Status != nil {
		if len(spec.Status.VersionArgs) > 0 {
			report.Results = append(report.Results, checkVersion(ctx, spec))
		}

		if spec.Status.ConfigDir != "" {
			report.Results = append(report.Results, checkConfigDir(spec))
		}

		if spec.Status.AuthCheck != nil && spec.Status.AuthCheck.Path != "" {
			report.Results = append(report.Results, checkAuthFile(spec))
		}
	}

	return report, nil
}

// CheckAllHealth runs health checks for every registered provider concurrently.
func CheckAllHealth(ctx context.Context) []*HealthReport {
	names := ProviderNames()
	reports := make([]*HealthReport, len(names))

	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)

		go func(i int, name string) {
			defer wg.Done()

			report, err := CheckHealth(ctx, name)
			if err != nil {
				return // leaves reports[i] as nil
			}

			reports[i] = report
		}(i, name)
	}

	wg.Wait()

	// Filter out nil entries (failed lookups).
	filtered := make([]*HealthReport, 0, len(names))

	for _, r := range reports {
		if r != nil {
			filtered = append(filtered, r)
		}
	}

	return filtered
}

func checkBinary(spec *harnesstype.ProviderSpec) HealthResult {
	path, err := exec.LookPath(spec.Binary)
	if err != nil {
		result := HealthResult{
			Check:   "Binary",
			Message: fmt.Sprintf("%s not found in PATH", spec.Binary),
			Status:  HealthFail,
		}

		if spec.Status != nil && spec.Status.InstallHint != "" {
			result.Detail = spec.Status.InstallHint
		}

		return result
	}

	return HealthResult{
		Check:   "Binary",
		Message: fmt.Sprintf("%s found at %s", spec.Binary, path),
		Status:  HealthPass,
	}
}

func checkVersion(ctx context.Context, spec *harnesstype.ProviderSpec) HealthResult {
	//nolint:gosec // args come from embedded YAML, not user input
	cmd := exec.CommandContext(ctx, spec.Binary, spec.Status.VersionArgs...)

	out, err := cmd.Output()
	if err != nil {
		return HealthResult{
			Check:   "Version",
			Message: fmt.Sprintf("failed to get version: %v", err),
			Status:  HealthWarn,
		}
	}

	// Extract first line of output.
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]) //nolint:mnd // split into at most 2 parts

	return HealthResult{
		Check:   "Version",
		Message: version,
		Status:  HealthPass,
	}
}

func checkConfigDir(spec *harnesstype.ProviderSpec) HealthResult {
	dir := expandTilde(spec.Status.ConfigDir)

	if _, err := os.Stat(dir); err != nil {
		return HealthResult{
			Check:   "Config",
			Message: fmt.Sprintf("%s not found", spec.Status.ConfigDir),
			Status:  HealthWarn,
		}
	}

	return HealthResult{
		Check:   "Config",
		Message: spec.Status.ConfigDir,
		Status:  HealthPass,
	}
}

func checkAuthFile(spec *harnesstype.ProviderSpec) HealthResult {
	path := expandTilde(spec.Status.AuthCheck.Path)
	desc := spec.Status.AuthCheck.Description

	if desc == "" {
		desc = "Auth"
	}

	if _, err := os.Stat(path); err != nil {
		return HealthResult{
			Check:   desc,
			Message: fmt.Sprintf("%s not found", spec.Status.AuthCheck.Path),
			Status:  HealthWarn,
		}
	}

	return HealthResult{
		Check:   desc,
		Message: spec.Status.AuthCheck.Path,
		Status:  HealthPass,
	}
}

// expandTilde replaces a leading ~/ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, path[2:])
}
