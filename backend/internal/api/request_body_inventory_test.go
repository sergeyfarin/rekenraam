package api

import (
	"encoding/csv"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// Follow local calls (including generic correctionRoute[T] and handler
// closures), rather than assuming that every documented JSON body is decoded.
func cappedJSONOperations(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	calls := map[string][]string{}
	var routes *ast.FuncDecl
	callNames := func(node ast.Node) []string {
		var names []string
		ast.Inspect(node, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			expression := call.Fun
			switch generic := expression.(type) {
			case *ast.IndexExpr:
				expression = generic.X
			case *ast.IndexListExpr:
				expression = generic.X
			}
			if name, ok := expression.(*ast.Ident); ok {
				names = append(names, name.Name)
			}
			return true
		})
		return names
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		require.NoError(t, err)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, ok := selector.X.(*ast.Ident)
				if ok && receiver.Name == "json" && selector.Sel.Name == "NewDecoder" {
					assert.Contains(t, []string{"decodeJSONBody", "decodeOptionalJSONBody"}, function.Name.Name,
						"API JSON readers must use the capped shared decoder")
				}
				return true
			})
			calls[function.Name.Name] = callNames(function.Body)
			if function.Name.Name == "RegisterRoutesWithAuth" {
				routes = function
			}
		}
	}
	require.NotNil(t, routes)
	result := map[string][]string{}
	ast.Inspect(routes.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "HandleFunc" {
			return true
		}
		require.Len(t, call.Args, 2)
		literal, ok := call.Args[0].(*ast.BasicLit)
		require.True(t, ok, "route pattern must be literal; update inventory for a new registration shape")
		pattern, err := strconv.Unquote(literal.Value)
		require.NoError(t, err)
		visited := map[string]bool{}
		decoders := map[string]bool{}
		var visit func(string)
		visit = func(name string) {
			if visited[name] {
				return
			}
			visited[name] = true
			if name == "decodeJSONBody" || name == "decodeOptionalJSONBody" {
				decoders[name] = true
				return
			}
			for _, next := range calls[name] {
				visit(next)
			}
		}
		for _, name := range callNames(call.Args[1]) {
			visit(name)
		}
		if len(decoders) > 0 {
			for decoder := range decoders {
				result[pattern] = append(result[pattern], decoder)
			}
			sort.Strings(result[pattern])
		}
		return true
	})
	require.NotEmpty(t, result)
	// The shared status handler branches on a fixed action supplied by these
	// three wrappers. Ignore never reads a body; link is required, accept optional.
	for _, action := range []string{"accept", "link", "ignore"} {
		pattern := "POST /api/v1/investments/event-suggestions/{suggestion_id}/" + action
		require.Equal(t, []string{"decodeJSONBody", "decodeOptionalJSONBody"}, result[pattern], "review action refinement when this helper changes")
		switch action {
		case "accept":
			result[pattern] = []string{"decodeOptionalJSONBody"}
		case "link":
			result[pattern] = []string{"decodeJSONBody"}
		case "ignore":
			delete(result, pattern)
		}
	}
	return result
}

func readBodyContractYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &document))
	return document
}

func TestCappedJSONOperationsDeclare413(t *testing.T) {
	t.Parallel()
	capped := cappedJSONOperations(t)
	root := "../../../api/openapi"
	spec := readBodyContractYAML(t, root+"/openapi.yaml")
	var rows [][]string
	for path, value := range spec["paths"].(map[string]any) {
		reference := value.(map[string]any)["$ref"].(string)
		pathFile := filepath.Join(root, reference)
		for method, value := range readBodyContractYAML(t, pathFile) {
			if !strings.Contains(" get post put patch delete head options ", " "+method+" ") {
				continue
			}
			operation := value.(map[string]any)
			pattern := strings.ToUpper(method) + " " + path
			decoders := capped[pattern]
			responses := operation["responses"].(map[string]any)
			response, declared := responses["413"]
			if body, ok := operation["requestBody"].(map[string]any); ok {
				if content, ok := body["content"].(map[string]any); ok {
					if _, jsonBody := content["application/json"]; jsonBody {
						assert.NotEmpty(t, decoders, "documented JSON consumer must use shared decoder: %s", pattern)
					}
				}
			}
			if len(decoders) > 0 {
				require.True(t, declared, "%s must declare the capped decoder's refusal", pattern)
				if path == "/api/v1/imports" {
					assert.NotContains(t, response.(map[string]any), "$ref", "mixed JSON/multipart keeps its distinct limits")
				} else {
					assert.Equal(t, "../openapi.yaml#/components/responses/JSONBodyTooLarge", response.(map[string]any)["$ref"], pattern)
				}
				delete(capped, pattern)
			} else if declared {
				assert.Equal(t, "/api/v1/imports/analyze", path, "unexpected 413 claim on uncapped operation %s", pattern)
			}
			rows = append(rows, []string{strings.ToUpper(method), path, strings.Join(decoders, " + "), reference, strconv.FormatBool(declared)})
		}
	}
	assert.Empty(t, capped, "every capped registered operation must be documented")
	sort.Slice(rows, func(i, j int) bool { return strings.Join(rows[i], " ") < strings.Join(rows[j], " ") })
	// Optional body-free evidence output for the dated delivery report.
	if output := os.Getenv("REKENRAAM_BODY_INVENTORY"); output != "" {
		file, err := os.Create(output)
		require.NoError(t, err)
		defer file.Close()
		writer := csv.NewWriter(file)
		require.NoError(t, writer.Write([]string{"method", "path", "decoders", "path_file", "declares_413"}))
		require.NoError(t, writer.WriteAll(rows))
	}
	t.Logf("mapped %d documented operations", len(rows))
}
