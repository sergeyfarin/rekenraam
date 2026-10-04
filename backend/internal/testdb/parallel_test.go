package testdb

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// T-125: the application and database suites dominate the race gate, and
// every ordinary test there owns its own copied database. A top-level test
// that does not call t.Parallel() runs alone on one core, and under -race
// that serial share was 654 of the application package's 866 seconds. New
// tests must opt in, or say why not with a `// serial: <reason>` comment
// directly above the function (process environment, working directory or
// other process-wide state).
func TestIntegrationSuitesRunTestsInParallel(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"../app", "../db"} {
		files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") ||
					fn.Name.Name == "TestMain" || fn.Body == nil || len(fn.Body.List) == 0 {
					continue
				}
				if fn.Doc != nil && strings.Contains(fn.Doc.Text(), "serial:") {
					continue
				}
				if !callsParallel(fn.Body.List[0]) {
					t.Errorf("%s: %s must call t.Parallel() first or carry a `// serial: <reason>` comment",
						fset.Position(fn.Pos()), fn.Name.Name)
				}
			}
		}
	}
}

func callsParallel(stmt ast.Stmt) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Parallel"
}
