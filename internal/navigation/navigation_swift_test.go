package navigation

import (
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestSwiftModuleImportsNeverGuessFilesOrCrossTargetCalls(t *testing.T) {
	root := t.TempDir()
	mainFile := filepath.Join(root, "Sources", "App", "main.swift")
	otherFile := filepath.Join(root, "Sources", "Other", "helpers.swift")
	sources := []TextSource{
		{Path: mainFile, Text: "import Other\nfunc local() {}\nfunc run() { local(); helper(); Other.helper() }\n"},
		{Path: otherFile, Text: "func helper() {}\n"},
	}
	analysis, stats := NewGraphEngine(BuildOptions{DisableCache: true}).BuildTextSources(sources)
	if stats.Parsed != 2 || stats.Failed != 0 || stats.Recovered != 0 {
		t.Fatalf("Swift parse stats=%+v", stats)
	}
	graph := analysis.Graph()
	if len(graph.Imports) != 1 || graph.Imports[0].ImportPath != "Other" || len(graph.Imports[0].TargetPaths) != 0 {
		t.Fatalf("unproven Swift module target: %+v", graph.Imports)
	}
	assertSwiftCallsStayLocal(t, graph, mainFile)
}

func assertSwiftCallsStayLocal(t *testing.T, graph parser.NavigationGraph, mainFile string) {
	t.Helper()
	var local, unresolved int
	for _, call := range graph.Calls {
		if filepath.Clean(call.Path) != filepath.Clean(mainFile) {
			continue
		}
		switch call.Display {
		case "local":
			if call.TargetID == "" {
				t.Fatalf("same-file call unresolved: %+v", call)
			}
			local++
		case "helper", "Other.helper":
			if call.TargetID != "" {
				t.Fatalf("cross-module call guessed a target: %+v", call)
			}
			unresolved++
		}
	}
	if local != 1 || unresolved != 2 {
		t.Fatalf("Swift calls=%+v", graph.Calls)
	}
}
