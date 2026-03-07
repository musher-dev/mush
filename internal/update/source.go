package update

import (
	"path/filepath"
	"strings"
)

// InstallSource identifies how mush was installed.
type InstallSource string

const (
	InstallSourceUnknown    InstallSource = "unknown"
	InstallSourceStandalone InstallSource = "standalone"
	InstallSourceHomebrew   InstallSource = "homebrew"
)

// DetectInstallSource infers the installation source from the executable path.
func DetectInstallSource(binaryPath string) InstallSource {
	paths := []string{binaryPath}
	if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
		paths = append(paths, resolved)
	}

	for _, path := range paths {
		norm := strings.ToLower(filepath.ToSlash(path))
		if strings.Contains(norm, "/cellar/mush/") || strings.Contains(norm, "/homebrew/cellar/mush/") {
			return InstallSourceHomebrew
		}
	}

	if strings.TrimSpace(binaryPath) != "" {
		return InstallSourceStandalone
	}

	return InstallSourceUnknown
}

// AutoApplyAllowed reports whether background auto-apply is allowed for a source.
func AutoApplyAllowed(source InstallSource) bool {
	return source != InstallSourceHomebrew
}

// UpgradeHint returns a package-manager command when known.
func UpgradeHint(source InstallSource) string {
	switch source {
	case InstallSourceHomebrew:
		return "brew upgrade mush"
	default:
		return ""
	}
}
