package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func parseCmdMushFiles(t *testing.T) map[string]*ast.File {
	t.Helper()

	paths, err := filepath.Glob("cmd/mush/*.go")
	if err != nil {
		t.Fatalf("glob cmd/mush: %v", err)
	}

	files := make(map[string]*ast.File, len(paths))
	fset := token.NewFileSet()

	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
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
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "/main.go") {
			continue
		}

		if containsSelectorCall(file, "os", "Exit") {
			t.Errorf("%s uses os.Exit; return CLIError instead", path)
		}
	}
}

func TestCmdPackageDoesNotUseOutputDefaultOutsideMain(t *testing.T) {
	for path, file := range parseCmdMushFiles(t) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "/main.go") {
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
