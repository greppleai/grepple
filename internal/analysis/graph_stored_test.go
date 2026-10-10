package analysis

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestStoredGraphColdWarmParityAndInvalidation(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(parser.NavigationCacheDirectoryEnv, directory)
	sources := []Source{{Path: "snapshot.go", Content: []byte("package example\nfunc root(){leaf()}\nfunc leaf(){}\n")}}
	assertStoredGraphDirections(t, sources)
	sources[0].Content = []byte("package example\nfunc root(){leaf();leaf()}\nfunc leaf(){}\n")
	report, err := BuildStoredGraph(sources, 0, GraphQuery{Direction: "callers", Depth: 1, Symbol: "leaf"})
	if err != nil || len(report.Calls) != 2 {
		t.Fatalf("stale source cache: calls=%d err=%v", len(report.Calls), err)
	}
	paths, err := filepath.Glob(filepath.Join(directory, "query-ready", "*.gnav.languages-*", "*.gnav"))
	if err != nil || len(paths) < 2 {
		t.Fatalf("missing generations: %v %v", paths, err)
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := BuildStoredGraph(sources, 0, GraphQuery{Direction: "callers", Depth: 1, Symbol: "leaf"}); err != nil {
		t.Fatalf("corruption recovery: %v", err)
	}
}

func assertStoredGraphDirections(t *testing.T, sources []Source) {
	t.Helper()
	for _, direction := range []string{"callers", "callees", "dependencies", "dependents", "impact"} {
		request := GraphQuery{Direction: direction, Depth: 2, Symbol: "leaf"}
		universe, err := NewUniverse(sources, 0)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := BuildGraph(universe, &request)
		universe.Close()
		if err != nil {
			t.Fatal(err)
		}
		for run := 0; run < 2; run++ {
			actual, err := BuildStoredGraph(sources, 0, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(reference, actual) {
				t.Fatalf("%s run %d changed output", direction, run)
			}
		}
	}
}
