package prompt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptPackageUsesStdinFDForTTYChecks(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob prompt files: %v", err)
	}

	fset := token.NewFileSet()

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

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
