package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildNavigationGraphResolvesCallsAndUsesRelativePaths(t *testing.T) {
	root := t.TempDir()
	writeNavigationGraphFile(t, root, "main.go", "package sample\nfunc Run(){ helper() }\n")
	writeNavigationGraphFile(t, root, "helper.go", "package sample\nfunc helper() {}\n")
	chdir(t, root)
	graph := BuildNavigationGraph([]string{"main.go", "helper.go"})
	if len(graph.Declarations) != 2 || len(graph.Calls) != 1 {
		t.Fatalf("graph=%#v", graph)
	}
	call := graph.Calls[0]
	if call.TargetID == "" || call.Confidence != "unique-terminal" || len(call.CandidateTargetIDs) != 0 {
		t.Fatalf("call=%#v", call)
	}
	for _, declaration := range graph.Declarations {
		if filepath.IsAbs(declaration.Path) {
			t.Fatalf("declaration path is absolute: %#v", declaration)
		}
	}
}

func TestBuildNavigationGraphReportsDeterministicCandidateTargetIDs(t *testing.T) {
	root := t.TempDir()
	writeNavigationGraphFile(t, root, "main.go", "package sample\nfunc Run(){ helper() }\n")
	writeNavigationGraphFile(t, root, "a.go", "package sample\nfunc helper() {}\n")
	writeNavigationGraphFile(t, root, "b.go", "package sample\nfunc helper() {}\n")
	chdir(t, root)
	first := BuildNavigationGraph([]string{"main.go", "b.go", "a.go"})
	second := BuildNavigationGraph([]string{"a.go", "main.go", "b.go"})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("graphs differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first.Calls) != 1 || first.Calls[0].Confidence != "candidate" || first.Calls[0].TargetID != "" || len(first.Calls[0].CandidateTargetIDs) != 2 {
		t.Fatalf("call=%#v", first.Calls)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("invalid JSON: %s", encoded)
	}
}

func TestBuildNavigationGraphReportsSourceCompleteness(t *testing.T) {
	root := t.TempDir()
	writeNavigationGraphFile(t, root, "valid.go", "package sample\nfunc Valid() {}\n")
	writeNavigationGraphFile(t, root, "recovered.go", "package sample\nfunc Recovered( {\n")
	writeNavigationGraphFile(t, root, "binary.go", "package sample\x00")
	writeNavigationGraphFile(t, root, "invalid.go", string([]byte{0xff, 0xfe}))
	writeNavigationGraphFile(t, root, "notes.md", "# not navigation source\n")
	chdir(t, root)

	graph, stats := BuildNavigationGraphWithStats([]string{"valid.go", "recovered.go", "binary.go", "invalid.go", "notes.md"})
	if stats != (NavigationSourceStats{Attempted: 5, Parsed: 2, Skipped: 2, Failed: 1, Recovered: 1}) {
		t.Fatalf("stats=%#v", stats)
	}
	if len(graph.Declarations) == 0 {
		t.Fatalf("valid source was omitted: %#v", graph)
	}
}

func writeNavigationGraphFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
