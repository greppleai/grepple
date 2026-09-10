package parser

import "testing"

func TestBuildNavigationGraphRetainsSourceIdentity(t *testing.T) {
	content := "package sample\nfunc Start() { Finish() }\nfunc Finish() {}\n"
	graph := BuildNavigationGraph(content, "go", "sample/main.go")
	if len(graph.Declarations) != 2 || len(graph.Calls) != 1 {
		t.Fatalf("graph shape = %d declarations, %d calls: %+v", len(graph.Declarations), len(graph.Calls), graph)
	}
	for _, declaration := range graph.Declarations {
		if declaration.Language != "go" || declaration.Path != "sample/main.go" {
			t.Fatalf("declaration source identity = %+v", declaration)
		}
	}
	call := graph.Calls[0]
	if call.Name != "Finish" || call.Language != "go" || call.Path != "sample/main.go" {
		t.Fatalf("call = %+v", call)
	}
	if graph.Declarations[0].ID == "" || call.ID == "" || call.CallerID != graph.Declarations[0].ID {
		t.Fatalf("stable graph identities were not linked: %+v", graph)
	}
	repeated := BuildNavigationGraph(content, "go", "sample/main.go")
	if repeated.Declarations[0].ID != graph.Declarations[0].ID || repeated.Calls[0].ID != call.ID {
		t.Fatalf("graph identities are not deterministic: first=%+v repeated=%+v", graph, repeated)
	}
}

func TestNavigationCompatibilityUsesNormalizedGraph(t *testing.T) {
	content := "function start(): void { finish() }\nfunction finish(): void {}\n"
	graph := BuildNavigationGraph(content, "typescript", "")
	declarations, calls := Navigation(content, "typescript")
	if len(declarations) != len(graph.Declarations) || len(calls) != len(graph.Calls) {
		t.Fatalf("compatibility result differs from graph: graph=%+v declarations=%+v calls=%+v", graph, declarations, calls)
	}
}
