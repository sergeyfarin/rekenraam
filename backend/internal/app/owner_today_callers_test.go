package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// #605 (bug class 83). A service that pins the owner day with referenceContext
// validates, defaults and answers on that day through pinnedOwnerDay; a second
// OwnerTodayAt reading can fall after midnight. Every remaining caller of
// OwnerTodayAt is listed here with why it has no pinned day to share, so a new
// caller is classified rather than copied.
var ownerTodayCallers = map[string]string{
	"settings.go:OwnerToday":                              "the session's owner_today for the auth API; no reference read to share a day with",
	"institutions.go:CreateInstitution":                   "pins nothing; reads the clock once and answers with the version it inserted",
	"investments.go:SaveDividendDefault":                  "pins nothing; reached only from the API and answers with the saved row",
	"investments.go:SaveAutomationRules":                  "pins nothing; reached only from the API and answers with the saved rules",
	"pricing.go:SaveSourceAssignment":                     "pins nothing; reached only from the API and answers with the saved assignment",
	"ledger.go:CategoryTotals":                            "pins nothing; its reads take the before date explicitly",
	"reconciliation.go:ChangePostingReconciliationStatus": "pins nothing; cleared_on is a stored date, not a dated reference selection",
	"reconciliation_age.go:AccountReconciliationAge":      "reads the clock once and pins that day itself with WithReferenceDate",
	"reconciliation_age.go:AccountStatementCommodities":   "reads the clock once and pins that day itself with WithReferenceDate",
	"pricing_refresh.go:sourcesForFXPair":                 "now is the instant runFXRefresh pinned (startedAt), so it resolves the same day",
}

func TestOwnerTodayCallersHaveNoPinnedDay(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			calls := map[string]bool{}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
						calls[selector.Sel.Name] = true
					}
				}
				return true
			})
			if !calls["OwnerTodayAt"] {
				continue
			}
			key := file + ":" + fn.Name.Name
			seen[key] = true
			if calls["referenceContext"] {
				t.Errorf("%s pins the owner day and reads it again with OwnerTodayAt: use pinnedOwnerDay(ctx)", fset.Position(fn.Pos()))
				continue
			}
			if _, listed := ownerTodayCallers[key]; !listed {
				t.Errorf("%s calls OwnerTodayAt: pin with referenceContext and use pinnedOwnerDay(ctx), or list it here with why it has no pinned day", fset.Position(fn.Pos()))
			}
		}
	}
	for key := range ownerTodayCallers {
		if !seen[key] {
			t.Errorf("listed caller %s no longer calls OwnerTodayAt: remove it from the list", key)
		}
	}
}
