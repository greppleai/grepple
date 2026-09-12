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
