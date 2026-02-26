package main

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const moduleRoot = "github.com/musher-dev/mush"

// Layer assignments for every internal package. When a new package is added
// under internal/, it must be classified here — otherwise
// TestAllInternalPackagesClassified will fail with a helpful message.
var (
	featureOrchestration = map[string]bool{
		moduleRoot + "/internal/harness": true,
		moduleRoot + "/internal/wizard":  true,
		moduleRoot + "/internal/doctor":  true,
		moduleRoot + "/internal/prompt":  true,
		moduleRoot + "/internal/output":  true,
		moduleRoot + "/internal/bundle":  true,
	}

	platformCore = map[string]bool{
		moduleRoot + "/internal/client":        true,
		moduleRoot + "/internal/auth":          true,
		moduleRoot + "/internal/config":        true,
		moduleRoot + "/internal/update":        true,
		moduleRoot + "/internal/worker":        true,
		moduleRoot + "/internal/errors":        true,
		moduleRoot + "/internal/buildinfo":     true,
		moduleRoot + "/internal/terminal":      true,
		moduleRoot + "/internal/paths":         true,
		moduleRoot + "/internal/ansi":          true,
		moduleRoot + "/internal/observability": true,
		moduleRoot + "/internal/transcript":    true,
		moduleRoot + "/internal/testutil":      true,
	}

	presentationPkgs = map[string]bool{
		moduleRoot + "/internal/output": true,
		moduleRoot + "/internal/prompt": true,
	}
)

// loadAllPackages loads all packages under the module root with import information.
func loadAllPackages(t *testing.T) []*packages.Package {
	t.Helper()

	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Tests: true,
	}

	pkgs, err := packages.Load(cfg, moduleRoot+"/...")
	if err != nil {
		t.Fatalf("loading packages: %v", err)
	}

	return pkgs
}

// isInternal returns true if the package path is under internal/.
func isInternal(path string) bool {
	return strings.HasPrefix(path, moduleRoot+"/internal/")
}

// internalBase returns the base internal package path (e.g.
// "github.com/musher-dev/mush/internal/auth" from a deeper subpackage).
// It strips any ".test" suffix added by the go/packages test loader.
func internalBase(path string) string {
	suffix := strings.TrimPrefix(path, moduleRoot+"/internal/")
	parts := strings.SplitN(suffix, "/", 2)

	base := parts[0]

	// Strip ".test" suffix from test package IDs and "_test" from external test packages.
	base = strings.TrimSuffix(base, ".test")
	base = strings.TrimSuffix(base, "_test")

	return moduleRoot + "/internal/" + base
}

// isTestPackage returns true if the package is a test-only variant
// (has a _test suffix in the package name loaded by go/packages).
func isTestPackage(pkg *packages.Package) bool {
	return strings.HasSuffix(pkg.ID, ".test") ||
		strings.HasSuffix(pkg.ID, ".test]") ||
		strings.Contains(pkg.ID, " [") // test variant like "pkg [pkg.test]"
}

// TestAllInternalPackagesClassified ensures every internal/ package is
// explicitly assigned to a layer. When a developer adds a new package, this
// test fails with instructions to classify it.
func TestAllInternalPackagesClassified(t *testing.T) {
	pkgs := loadAllPackages(t)

	seen := map[string]bool{}

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) {
			continue
		}

		base := internalBase(pkg.PkgPath)
		if seen[base] {
			continue
		}

		seen[base] = true

		if !featureOrchestration[base] && !platformCore[base] {
			t.Errorf("internal package %s is not classified in architecture_test.go layer maps.\n"+
				"Add it to featureOrchestration or platformCore.", base)
		}
	}
}

// TestPlatformPackagesDoNotImportPresentation verifies that platform/core
// packages do not import output or prompt.
func TestPlatformPackagesDoNotImportPresentation(t *testing.T) {
	pkgs := loadAllPackages(t)

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) {
			continue
		}

		base := internalBase(pkg.PkgPath)
		if !platformCore[base] {
			continue
		}

		// Skip test-only package variants.
		if isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if presentationPkgs[imp] {
				t.Errorf("%s imports presentation package %s — platform/core must not depend on output or prompt",
					pkg.PkgPath, imp)
			}
		}
	}
}

// TestInternalPackagesDoNotImportCmd verifies no internal/ package imports cmd/mush.
func TestInternalPackagesDoNotImportCmd(t *testing.T) {
	pkgs := loadAllPackages(t)

	cmdPrefix := moduleRoot + "/cmd/"

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) {
			continue
		}

		if isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if strings.HasPrefix(imp, cmdPrefix) {
				t.Errorf("%s imports cmd package %s — internal packages must not import cmd/",
					pkg.PkgPath, imp)
			}
		}
	}
}

// TestDoctorDoesNotImportOutput verifies that internal/doctor does not import
// internal/output, keeping diagnostics output-agnostic.
func TestDoctorDoesNotImportOutput(t *testing.T) {
	pkgs := loadAllPackages(t)

	doctorPkg := moduleRoot + "/internal/doctor"
	outputPkg := moduleRoot + "/internal/output"

	for _, pkg := range pkgs {
		if pkg.PkgPath != doctorPkg {
			continue
		}

		if isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if imp == outputPkg {
				t.Errorf("internal/doctor imports internal/output — doctor must remain output-agnostic")
			}
		}
	}
}

// TestTestutilNotImportedByProductionCode verifies that internal/testutil is
// only imported by test package variants.
func TestTestutilNotImportedByProductionCode(t *testing.T) {
	pkgs := loadAllPackages(t)

	testutilPkg := moduleRoot + "/internal/testutil"

	for _, pkg := range pkgs {
		if isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if imp == testutilPkg {
				t.Errorf("%s imports internal/testutil — testutil is for tests only",
					pkg.PkgPath)
			}
		}
	}
}

// TestNoCrossLayerFeatureImports verifies that feature/orchestration packages
// do not import each other laterally. Each feature package should depend only
// on platform/core packages, not on sibling features.
func TestNoCrossLayerFeatureImports(t *testing.T) {
	// Allowed lateral imports — explicitly blessed cross-feature dependencies.
	// Add entries here only when a lateral dependency is architecturally justified.
	allowed := map[string]map[string]bool{
		// harness depends on output for the watch UI.
		moduleRoot + "/internal/harness": {
			moduleRoot + "/internal/output": true,
		},
		// wizard depends on prompt and output for interactive onboarding.
		moduleRoot + "/internal/wizard": {
			moduleRoot + "/internal/prompt": true,
			moduleRoot + "/internal/output": true,
		},
		// bundle depends on output for install progress and harness for provider types.
		moduleRoot + "/internal/bundle": {
			moduleRoot + "/internal/output":  true,
			moduleRoot + "/internal/harness": true,
		},
		// doctor depends on harness for provider availability checks.
		moduleRoot + "/internal/doctor": {
			moduleRoot + "/internal/harness": true,
		},
		// prompt depends on output for styled user interaction.
		moduleRoot + "/internal/prompt": {
			moduleRoot + "/internal/output": true,
		},
	}

	pkgs := loadAllPackages(t)

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) {
			continue
		}

		base := internalBase(pkg.PkgPath)
		if !featureOrchestration[base] {
			continue
		}

		if isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if !isInternal(imp) {
				continue
			}

			impBase := internalBase(imp)
			if !featureOrchestration[impBase] {
				continue // importing platform/core is fine
			}

			if impBase == base {
				continue // importing self (subpackage) is fine
			}

			if allowed[base][impBase] {
				continue
			}

			t.Errorf("%s imports sibling feature package %s — feature packages should not import each other laterally.\n"+
				"If this dependency is intentional, add it to the allowed map in TestNoCrossLayerFeatureImports.",
				pkg.PkgPath, imp)
		}
	}
}
