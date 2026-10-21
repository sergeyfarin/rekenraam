// Record syntactic definitions/calls of selected invariant gates for #284.
// This deliberately does not claim receiver resolution or a complete call graph.
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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
	targetFile := flag.String("targets", "", "JSON array of function/method names")
	flag.Parse()
	if err := run(*root, *targetFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, targetFile string) error {
	data, err := os.ReadFile(targetFile)
	if err != nil {
		return err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	targets := map[string]bool{}
	for _, name := range names {
		targets[name] = true
	}
	var rows [][]string
	fset := token.NewFileSet()
	files, functions := 0, 0
	err = filepath.WalkDir(filepath.Join(root, "backend"), func(path string, entry os.DirEntry, walkErr error) error {
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
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			functions++
			scope := fn.Name.Name
			if fn.Recv != nil {
				var receiver bytes.Buffer
				if err := format.Node(&receiver, fset, fn.Recv.List[0].Type); err != nil {
					return err
				}
				scope = receiver.String() + "." + scope
			}
			add := func(kind, symbol, expression string, node ast.Node) {
				rows = append(rows, []string{kind, filepath.ToSlash(relative), strconv.Itoa(fset.Position(node.Pos()).Line),
					strconv.Itoa(fset.Position(node.End()).Line), scope, symbol, expression})
			}
			if targets[fn.Name.Name] {
				add("definition", fn.Name.Name, scope, fn)
			}
			if fn.Body == nil {
				continue
			}
			called := map[token.Pos]bool{}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				fun := call.Fun
				for {
					switch indexed := fun.(type) {
					case *ast.IndexExpr:
						fun = indexed.X
					case *ast.IndexListExpr:
						fun = indexed.X
					default:
						goto base
					}
				}
			base:
				name := ""
				switch value := fun.(type) {
				case *ast.Ident:
					name = value.Name
					called[value.Pos()] = true
				case *ast.SelectorExpr:
					name = value.Sel.Name
					called[value.Sel.Pos()] = true
				}
				if targets[name] {
					var expression bytes.Buffer
					if format.Node(&expression, fset, fun) == nil {
						add("call", name, expression.String(), call)
					}
				}
				return true
			})
			// Function-valued aliases (including generic writer dispatch) are not
			// direct calls. Keep their syntactic references visible for review.
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok && targets[ident.Name] && !called[ident.Pos()] {
					add("reference", ident.Name, ident.Name, ident)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Fail on misspelled gate names instead of silently publishing an empty row.
	for name := range targets {
		found := false
		for _, row := range rows {
			if row[0] == "definition" && row[5] == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no definition for selected gate %q", name)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return strings.Join(rows[i], "\x00") < strings.Join(rows[j], "\x00") })
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"kind", "file", "start_line", "end_line", "scope", "symbol", "expression"}); err != nil {
		return err
	}
	if err := w.WriteAll(rows); err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stderr, "files=%d functions=%d selected_gates=%d sites=%d\n", files, functions, len(targets), len(rows))
	return err
}
