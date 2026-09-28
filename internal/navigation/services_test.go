package navigation

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestNavigationPackageExportsOnlyServiceConstructors(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var exported []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, parseErr := goparser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.IsExported() {
				exported = append(exported, fn.Name.Name)
			}
		}
	}
	sort.Strings(exported)
	want := []string{"NewExternalDependencyService", "NewExternalResolutionService", "NewGraphEngine", "NewGraphOperations"}
	if !reflect.DeepEqual(exported, want) {
		t.Fatalf("package-level functions=%v; want only interface constructors %v", exported, want)
	}
}

func TestGraphServicesBuildAndProject(t *testing.T) {
	engine := NewGraphEngine(BuildOptions{DisableCache: true})
	analysis, stats := engine.BuildTextSources([]TextSource{{Path: "main.go", Text: "package sample\nfunc Target() {}\n"}})
	graph := analysis.Graph()
	if stats.Parsed != 1 || len(graph.Declarations) != 1 {
		t.Fatalf("stats=%+v declarations=%+v", stats, graph.Declarations)
	}
	operations := NewGraphOperations()
	filter, err := operations.NormalizeFilter(NavigationGraphFilter{})
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := operations.Filter(graph, filter)
	if err != nil || len(filtered.Declarations) != 1 {
		t.Fatalf("filter=%+v error=%v", filtered, err)
	}
	query, err := operations.Query(filtered, []string{filtered.Declarations[0].ID}, NavigationQueryCallees, 1)
	if err != nil || len(query.Declarations) != 1 {
		t.Fatalf("query=%+v error=%v", query, err)
	}
	if diff := operations.Diff(graph, graph); len(diff.ChangedDeclarations) != 0 || len(diff.ChangedCalls) != 0 {
		t.Fatalf("self diff=%+v", diff)
	}
	_ = operations.ResolutionStats(graph)
}
