package policy_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const moduleRoot = "github.com/musher-dev/mush"

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
		moduleRoot + "/internal/tui":           true,
		moduleRoot + "/internal/observability": true,
		moduleRoot + "/internal/transcript":    true,
		moduleRoot + "/internal/testutil":      true,
		moduleRoot + "/internal/safeio":        true,
		moduleRoot + "/internal/executil":      true,
		moduleRoot + "/internal/devhooks":      true,
		moduleRoot + "/internal/policy":        true,
		moduleRoot + "/internal/validate":      true,
	}

	presentationPkgs = map[string]bool{
		moduleRoot + "/internal/output": true,
		moduleRoot + "/internal/prompt": true,
	}
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func parseCmdMushFiles(t *testing.T) map[string]*ast.File {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "cmd", "mush", "*.go"))
	if err != nil {
		t.Fatalf("glob cmd/mush: %v", err)
	}

	files := make(map[string]*ast.File, len(paths))
	fset := token.NewFileSet()

	for _, path := range paths {
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		files[path] = file
	}

	return files
}

func parsePromptFiles(t *testing.T) map[string]*ast.File {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "prompt", "*.go"))
	if err != nil {
		t.Fatalf("glob prompt files: %v", err)
	}

	files := make(map[string]*ast.File, len(paths))
	fset := token.NewFileSet()

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		files[path] = file
	}

	return files
}

func selectorMatches(expr ast.Expr, pkgName, member string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == pkgName && sel.Sel.Name == member
}

func containsSelectorCall(file *ast.File, pkgName, member string) bool {
	found := false

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if selectorMatches(call.Fun, pkgName, member) {
			found = true
			return false
		}

		return true
	})

	return found
}

func fileContainsIdentifier(file *ast.File, identName string) bool {
	found := false

	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == identName {
			found = true
			return false
		}

		return true
	})

	return found
}

func loadAllPackages(t *testing.T) []*packages.Package {
	t.Helper()

	cfg := &packages.Config{
		Dir:   repoRoot(t),
		Mode:  packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Tests: true,
	}

	pkgs, err := packages.Load(cfg, moduleRoot+"/...")
	if err != nil {
		t.Fatalf("loading packages: %v", err)
	}

	return pkgs
}

func isInternal(path string) bool {
	return strings.HasPrefix(path, moduleRoot+"/internal/")
}

func internalBase(path string) string {
	suffix := strings.TrimPrefix(path, moduleRoot+"/internal/")
	parts := strings.SplitN(suffix, "/", 2)

	base := parts[0]
	base = strings.TrimSuffix(base, ".test")
	base = strings.TrimSuffix(base, "_test")

	return moduleRoot + "/internal/" + base
}

func isTestPackage(pkg *packages.Package) bool {
	return strings.HasSuffix(pkg.ID, ".test") ||
		strings.HasSuffix(pkg.ID, ".test]") ||
		strings.Contains(pkg.ID, " [")
}

func TestCmdPackageDoesNotUseFmtErrorf(t *testing.T) {
	for path, file := range parseCmdMushFiles(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			if selectorMatches(call.Fun, "fmt", "Errorf") {
				t.Errorf("%s uses fmt.Errorf; use clierrors.Wrap or clierrors.New instead", path)
				return false
			}

			return true
		})
	}
}

func TestCmdPackageDoesNotUseOsExitOutsideMain(t *testing.T) {
	for path, file := range parseCmdMushFiles(t) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, string(filepath.Separator)+"main.go") {
			continue
		}

		if containsSelectorCall(file, "os", "Exit") {
			t.Errorf("%s uses os.Exit; return CLIError instead", path)
		}
	}
}

func TestCmdPackageDoesNotUseOutputDefaultOutsideMain(t *testing.T) {
	for path, file := range parseCmdMushFiles(t) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, string(filepath.Separator)+"main.go") {
			continue
		}

		if containsSelectorCall(file, "output", "Default") {
			t.Errorf("%s uses output.Default(); use output.FromContext(cmd.Context()) instead", path)
		}
	}
}

func TestConfirmCallersCheckNoInput(t *testing.T) {
	for path, file := range parseCmdMushFiles(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		if !containsSelectorCall(file, "prompter", "Confirm") && !containsSelectorCall(file, "p", "Confirm") {
			hasConfirm := false

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Confirm" {
					hasConfirm = true
					return false
				}

				return true
			})

			if !hasConfirm {
				continue
			}
		}

		if !fileContainsIdentifier(file, "NoInput") {
			t.Errorf("%s uses Confirm() but does not reference NoInput", path)
		}
	}
}

func TestPromptPackageUsesStdinFDForTTYChecks(t *testing.T) {
	for path, file := range parsePromptFiles(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Fd" {
				return true
			}

			outer, ok := sel.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			pkg, ok := outer.X.(*ast.Ident)
			if ok && pkg.Name == "os" && outer.Sel.Name == "Stdout" {
				t.Errorf("%s uses os.Stdout.Fd(); use os.Stdin.Fd() for TTY checks", path)
				return false
			}

			return true
		})
	}
}

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

func TestPlatformPackagesDoNotImportPresentation(t *testing.T) {
	pkgs := loadAllPackages(t)

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) {
			continue
		}

		base := internalBase(pkg.PkgPath)
		if !platformCore[base] || isTestPackage(pkg) {
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

func TestInternalPackagesDoNotImportCmd(t *testing.T) {
	pkgs := loadAllPackages(t)
	cmdPrefix := moduleRoot + "/cmd/"

	for _, pkg := range pkgs {
		if !isInternal(pkg.PkgPath) || isTestPackage(pkg) {
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

func TestDoctorDoesNotImportOutput(t *testing.T) {
	pkgs := loadAllPackages(t)

	doctorPkg := moduleRoot + "/internal/doctor"
	outputPkg := moduleRoot + "/internal/output"

	for _, pkg := range pkgs {
		if pkg.PkgPath != doctorPkg || isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if imp == outputPkg {
				t.Errorf("internal/doctor imports internal/output — doctor must remain output-agnostic")
			}
		}
	}
}

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

func TestNoCrossLayerFeatureImports(t *testing.T) {
	allowed := map[string]map[string]bool{
		moduleRoot + "/internal/harness": {
			moduleRoot + "/internal/output": true,
		},
		moduleRoot + "/internal/wizard": {
			moduleRoot + "/internal/prompt": true,
			moduleRoot + "/internal/output": true,
		},
		moduleRoot + "/internal/bundle": {
			moduleRoot + "/internal/output":  true,
			moduleRoot + "/internal/harness": true,
		},
		moduleRoot + "/internal/doctor": {
			moduleRoot + "/internal/harness": true,
		},
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
		if !featureOrchestration[base] || isTestPackage(pkg) {
			continue
		}

		for imp := range pkg.Imports {
			if !isInternal(imp) {
				continue
			}

			impBase := internalBase(imp)
			if !featureOrchestration[impBase] || impBase == base || allowed[base][impBase] {
				continue
			}

			t.Errorf("%s imports sibling feature package %s — feature packages should not import each other laterally.\n"+
				"If this dependency is intentional, add it to the allowed map in TestNoCrossLayerFeatureImports.",
				pkg.PkgPath, imp)
		}
	}
}
