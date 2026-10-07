package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func parityFixture() parser.NavigationGraph {
	return parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{{ID: "root", Path: "a.go", Container: "pkg.Widget"}, {ID: "a", Path: "a.go"}, {ID: "b", Path: "b.go"}, {ID: "incoming", Path: "c.go"}, {ID: "other", Path: "d.go"}},
		Calls: []parser.NavigationCall{
			{ID: "root-a", CallerID: "root", TargetID: "a", Path: "a.go"},
			{ID: "a-b", CallerID: "a", TargetID: "b", Path: "a.go"},
			{ID: "b-root", CallerID: "b", TargetID: "root", Path: "b.go"},
			{ID: "incoming-root", CallerID: "incoming", CandidateTargetIDs: []string{"root", "other"}, Path: "c.go"},
			{ID: "unknown-target", CallerID: "root", TargetID: "missing", Path: "a.go"},
			{ID: "also-unknown", CallerID: "other", TargetID: "missing", Path: "d.go"},
			{ID: "unresolved", CallerID: "root", Path: "a.go"},
		},
		Fields:           []parser.NavigationField{{OwnerType: "pkg.Widget", Name: "x", Path: "types.go"}},
		TypeDeclarations: []parser.NavigationTypeDeclaration{{Name: "Widget", Path: "types.go"}},
		Imports:          []parser.NavigationImport{{Path: "a.go", ImportPath: "external"}},
		Exports:          []parser.NavigationExport{{Path: "b.go", Name: "Exported"}},
		TypeUsages:       []parser.NavigationTypeUsage{{CallerID: "root", Type: "Widget"}},
		MemberAccesses:   []parser.NavigationMemberAccess{{CallerID: "root", Member: "x"}},
		RepositoryRoots:  []string{"fixture"},
	}
}
func assertParity(t *testing.T, index queryIndex, graph parser.NavigationGraph) {
	t.Helper()
	for _, roots := range [][]string{{"root"}, {"root", "a"}, {"incoming"}, {"other"}} {
		for _, direction := range []navigation.NavigationQueryDirection{navigation.NavigationQueryCallers, navigation.NavigationQueryCallees, navigation.NavigationQueryDependencies, navigation.NavigationQueryDependents, navigation.NavigationQueryImpact} {
			for depth := 1; depth <= 10; depth++ {
				q := request{Roots: roots, Direction: direction, Depth: depth}
				want, err := navigation.NewGraphOperations().Query(graph, roots, direction, depth)
				if err != nil {
					t.Fatal(err)
				}
				got, err := index.Query(context.Background(), q)
				if err != nil {
					t.Fatal(err)
				}
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want)
				if string(a) != string(b) {
					t.Fatalf("query=%+v\ngot=%s\nwant=%s", q, a, b)
				}
			}
		}
	}
	for _, q := range []request{{Roots: []string{"unknown"}, Direction: navigation.NavigationQueryCallees, Depth: 1}, {Direction: navigation.NavigationQueryCallers, Depth: 1}, {Roots: []string{"root"}, Direction: "invalid", Depth: 1}} {
		if _, err := index.Query(context.Background(), q); err == nil {
			t.Fatalf("invalid request accepted: %+v", q)
		}
	}
}
func TestAdjacencyParity(t *testing.T) {
	graph := parityFixture()
	index := newAdjacency(graph)
	defer index.Close()
	assertParity(t, index, graph)
}
func TestDuckParity(t *testing.T) {
	if !duckDBEnabled {
		t.Skip("build with -tags duckdb")
	}
	graph := parityFixture()
	path := filepath.Join(t.TempDir(), "graph.duckdb")
	if _, err := buildDuck(path, graph); err != nil {
		t.Fatal(err)
	}
	index, err := openDuck(path)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	assertParity(t, index, graph)
}

func TestColumnDuckParity(t *testing.T) {
	if !duckDBEnabled {
		t.Skip("build with -tags duckdb")
	}
	graph := parityFixture()
	path := filepath.Join(t.TempDir(), "column.duckdb")
	if _, err := selectedColumnBuilder(path, graph); err != nil {
		t.Fatal(err)
	}
	index, err := selectedColumnOpener(path)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	assertParity(t, index, graph)
}
