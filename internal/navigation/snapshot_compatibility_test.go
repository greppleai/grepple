package navigation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestSnapshotFullGraphCompatibilityAndEmptyShapes(t *testing.T) {
	for _, graph := range []parser.NavigationGraph{{}, {Declarations: []parser.NavigationDeclaration{}, Calls: []parser.NavigationCall{}}, navigationQueryFixture()} {
		snapshot, err := NewGraphOperations().PrepareQuery(graph)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := snapshot.Graph()
		if err != nil {
			t.Fatal(err)
		}
		expectedJSON, _ := json.Marshal(graph)
		actualJSON, _ := json.Marshal(actual)
		if string(expectedJSON) != string(actualJSON) {
			t.Fatal("full compatibility output changed")
		}
	}
}

func TestGraphEngineBuildQueryFilesPreservesStatsAndGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc finish() {}\nfunc start() { finish() }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := NewGraphEngine(BuildOptions{DisableCache: true})
	graph, stats := engine.BuildFiles([]string{path})
	snapshot, actualStats, err := engine.BuildQueryFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if stats != actualStats || stats.Parsed != 1 {
		t.Fatal("source completeness changed")
	}
	assertSnapshotQueryParity(t, graph, snapshot, []string{graph.Declarations[0].ID}, NavigationQueryImpact, 2)
}
