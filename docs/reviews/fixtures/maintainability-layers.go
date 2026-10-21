// Syntactic layer signals for #284; no receiver or financial-type resolution.
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	root := flag.String("root", "", "fixed-SHA source directory")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string) error {
	var rows [][]string
	fset := token.NewFileSet()
	files := 0
	err := filepath.WalkDir(filepath.Join(root, "backend"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files++
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		aliases := map[string]string{}
		add := func(kind, scope, signal string, node ast.Node) {
			var expression bytes.Buffer
			if call, ok := node.(*ast.CallExpr); ok {
				node = call.Fun // Keep the call site, rather than duplicate whole SQL bodies.
			}
			_ = format.Node(&expression, fset, node)
			rows = append(rows, []string{relative, strconv.Itoa(fset.Position(node.Pos()).Line), kind, scope, signal, expression.String()})
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			alias := filepath.Base(name)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			aliases[alias] = name
			if strings.Contains(name, "/internal/") || name == "database/sql" || name == "math/big" {
				add("import", "", name, imp)
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if binary, ok := node.(*ast.BinaryExpr); ok && strings.Contains(relative, "/api/") {
					switch binary.Op {
					case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
						add("api-arithmetic", fn.Name.Name, binary.Op.String(), binary)
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && (ident.Name == "float64" || ident.Name == "float32") {
					add("float-cast", fn.Name.Name, ident.Name, call)
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				name := sel.Sel.Name
				if pkg, ok := sel.X.(*ast.Ident); ok {
					owner := aliases[pkg.Name]
					if strings.HasSuffix(owner, "/exact") || owner == "math/big" || owner == "github.com/shopspring/decimal" {
						add("exact-package-call", fn.Name.Name, owner+"."+name, call)
					}
					if owner == "strconv" && (name == "ParseFloat" || name == "FormatFloat") {
						add("float-call", fn.Name.Name, owner+"."+name, call)
					}
				}
				switch name {
				case "QueryContext", "QueryRowContext", "ExecContext", "BeginTx":
					add("sql-shaped-call", fn.Name.Name, name, call)
				case "AddInt64", "SubInt64", "MulDivRound", "SubScaled", "AddScaled", "Coefficient", "Int64":
					add("numeric-method-name", fn.Name.Name, name, call)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(rows, func(i, j int) bool { return strings.Join(rows[i], "\x00") < strings.Join(rows[j], "\x00") })
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"file", "line", "kind", "scope", "signal", "expression"}); err != nil {
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "files=%d sites=%d\n", files, len(rows))
	return nil
}
