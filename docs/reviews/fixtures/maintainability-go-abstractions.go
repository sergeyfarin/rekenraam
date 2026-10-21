// Type-resolved references for named interfaces and *Option(s) structs, #284.
// This indexes reference owners, not a call graph or dynamic wiring proof.
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

func main() {
	root := flag.String("root", "", "fixed-SHA backend module directory")
	flag.Parse()
	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	pkgs, err := packages.Load(&packages.Config{Dir: root, Fset: fset, Tests: true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps}, "./...")
	if err != nil {
		return err
	}
	if packages.PrintErrors(pkgs) != 0 {
		return fmt.Errorf("package loading failed")
	}
	type selected struct{ name, kind, declarationFile string }
	selectedObjects := map[string]selected{}
	key := func(obj types.Object) string {
		pos := fset.Position(obj.Pos())
		return pos.Filename + ":" + strconv.Itoa(pos.Offset)
	}
	for _, pkg := range pkgs {
		for _, obj := range pkg.TypesInfo.Defs {
			name, ok := obj.(*types.TypeName)
			if !ok || strings.HasSuffix(fset.Position(obj.Pos()).Filename, "_test.go") {
				continue
			}
			if _, parameter := name.Type().(*types.TypeParam); parameter {
				continue // Generic T constraints are not named interface abstractions.
			}
			kind := ""
			switch name.Type().Underlying().(type) {
			case *types.Interface:
				kind = "interface"
			case *types.Struct:
				if strings.HasSuffix(name.Name(), "Options") || strings.HasSuffix(name.Name(), "Option") {
					kind = "options-struct"
				}
			}
			if kind != "" {
				rel, err := filepath.Rel(root, fset.Position(obj.Pos()).Filename)
				if err != nil {
					return err
				}
				selectedObjects[key(obj)] = selected{name.Name(), kind, "backend/" + filepath.ToSlash(rel)}
			}
		}
	}
	rows := map[string][]string{}
	add := func(kind string, info selected, node ast.Node, scope, expression string) {
		pos := fset.Position(node.Pos())
		rel, err := filepath.Rel(root, pos.Filename)
		if err != nil || strings.HasPrefix(rel, "..") {
			return
		}
		row := []string{kind, "backend/" + filepath.ToSlash(rel), strconv.Itoa(pos.Line), info.kind, info.declarationFile, info.name, scope, expression}
		// Deduplicate recompilations of a package in its test executable.
		rows[kind+":"+pos.Filename+":"+strconv.Itoa(pos.Offset)+":"+info.name] = row
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				typeDecl, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range typeDecl.Specs {
					typ, ok := spec.(*ast.TypeSpec)
					if !ok || pkg.TypesInfo.Defs[typ.Name] == nil {
						continue
					}
					if info, ok := selectedObjects[key(pkg.TypesInfo.Defs[typ.Name])]; ok {
						var expression bytes.Buffer
						_ = format.Node(&expression, fset, typ.Type)
						add("definition", info, typ.Name, "", expression.String())
					}
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok {
					if obj := pkg.TypesInfo.Uses[ident]; obj != nil {
						if info, ok := selectedObjects[key(obj)]; ok {
							add("type-reference", info, ident, owner(file, ident.Pos()), ident.Name)
						}
					}
				}
				if selector, ok := node.(*ast.SelectorExpr); ok {
					selection := pkg.TypesInfo.Selections[selector]
					if selection == nil {
						return true
					}
					receiver := selection.Recv()
					if ptr, ok := receiver.(*types.Pointer); ok {
						receiver = ptr.Elem()
					}
					if named, ok := receiver.(*types.Named); ok {
						if info, ok := selectedObjects[key(named.Obj())]; ok {
							add("member-reference", info, selector, owner(file, selector.Pos()), selector.Sel.Name)
						}
					}
				}
				return true
			})
		}
	}
	ordered := make([][]string, 0, len(rows))
	for _, row := range rows {
		ordered = append(ordered, row)
	}
	sort.Slice(ordered, func(i, j int) bool { return strings.Join(ordered[i], "\x00") < strings.Join(ordered[j], "\x00") })
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"kind", "file", "line", "type_kind", "declaration_file", "type", "owner", "expression"}); err != nil {
		return err
	}
	if err := w.WriteAll(ordered); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "named_types=%d reference_rows=%d\n", len(selectedObjects), len(rows))
	return nil
}

func owner(file *ast.File, pos token.Pos) string {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Pos() <= pos && pos <= fn.End() {
			name := fn.Name.Name
			if fn.Recv != nil {
				var receiver bytes.Buffer
				_ = format.Node(&receiver, token.NewFileSet(), fn.Recv.List[0].Type)
				name = receiver.String() + "." + name
			}
			return name
		}
	}
	return "<package-declaration>"
}
