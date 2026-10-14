package navigation

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestLanguagePartitionsPreserveCrossLanguageQueryEvidence(t *testing.T) {
	graph := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "py", Name: "root", Language: "python", Path: "a.py", Container: "Widget"}, {ID: "go", Name: "leaf", Language: "go", Path: "b.go"}, {ID: "sh", Name: "other", Language: "shell", Path: "c.sh"}}, Calls: []parser.NavigationCall{{ID: "call", CallerID: "py", TargetID: "go", CandidateTargetIDs: []string{"go", "go"}, Language: "python", Path: "a.py"}}, Fields: []parser.NavigationField{{OwnerType: "Widget", Language: "ruby", Path: "d.rb"}}, TypeDeclarations: []parser.NavigationTypeDeclaration{{Name: "Widget", Language: "ruby", Path: "d.rb"}}}
	parts := NewGraphOperations().PartitionLanguages(graph)
	if len(parts) != 3 {
		t.Fatalf("parts=%d", len(parts))
	}
	for _, part := range parts {
		if part.Languages[0] != "go" {
			continue
		}
		assertLanguagePartitionQueryParity(t, graph, part.Graph)
	}
}
func assertLanguagePartitionQueryParity(t *testing.T, graph, part parser.NavigationGraph) {
	t.Helper()
	for _, direction := range []NavigationQueryDirection{"callers", "callees", "dependencies", "dependents", "impact"} {
		want, err := newQuerySnapshot(graph)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := newQuerySnapshot(part)
		if err != nil {
			t.Fatal(err)
		}
		a, err := actual.Query([]string{"py"}, direction, 2)
		if err != nil {
			t.Fatal(err)
		}
		w, err := want.Query([]string{"py"}, direction, 2)
		if err != nil || !reflect.DeepEqual(a, w) {
			t.Fatal("cross-language evidence changed")
		}
	}
}
func TestLanguagePartitionsRetainSharedFileContext(t *testing.T) {
	graph := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "one", Language: "svelte", Path: "page.svelte"}}, TypeDeclarations: []parser.NavigationTypeDeclaration{{Name: "Props", Language: "typescript", Path: "page.svelte"}}, Imports: []parser.NavigationImport{{Language: "javascript", Path: "page.svelte", ImportPath: "library"}}}
	parts := NewGraphOperations().PartitionLanguages(graph)
	if len(parts) != 1 || !reflect.DeepEqual(parts[0].Graph, graph) {
		t.Fatal("embedded source contexts split or changed")
	}
}
