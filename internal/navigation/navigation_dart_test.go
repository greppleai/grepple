package navigation

import (
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestDartImportsResolveOnlyExactSelectedRelativeLibraries(t *testing.T) {
	root := t.TempDir()
	mainFile := filepath.Join(root, "lib", "main.dart")
	helperFile := filepath.Join(root, "lib", "src", "helper.dart")
	unrelatedFile := filepath.Join(root, "lib", "unrelated.dart")
	sources := []TextSource{
		{Path: mainFile, Text: "import 'src/helper.dart' as helper;\nimport 'package:other/remote.dart' as remote;\nvoid run() { helper.format(); remote.format(); }\n"},
		{Path: helperFile, Text: "void format() {}\n"},
		{Path: unrelatedFile, Text: "void format() {}\n"},
	}
	analysis, stats := NewGraphEngine(BuildOptions{DisableCache: true}).BuildTextSources(sources)
	graph := analysis.Graph()
	if stats.Parsed != 3 || stats.Failed != 0 || stats.Recovered != 0 {
		t.Fatalf("Dart source stats=%+v", stats)
	}
	assertDartImports(t, graph, helperFile)
	assertDartCallTargets(t, graph)
}

func assertDartImports(t *testing.T, graph parser.NavigationGraph, helperFile string) {
	t.Helper()
	if len(graph.Imports) != 2 {
		t.Fatalf("Dart imports=%+v", graph.Imports)
	}
	for _, fact := range graph.Imports {
		switch fact.ImportPath {
		case "src/helper.dart":
			if len(fact.TargetPaths) != 1 || filepath.Clean(fact.TargetPaths[0]) != helperFile {
				t.Fatalf("relative Dart import=%+v", fact)
			}
		case "package:other/remote.dart":
			if len(fact.TargetPaths) != 0 {
				t.Fatalf("unproven Dart package import resolved: %+v", fact)
			}
		default:
			t.Fatalf("unexpected Dart import: %+v", fact)
		}
	}
}

func assertDartCallTargets(t *testing.T, graph parser.NavigationGraph) {
	t.Helper()
	resolved, unresolved := 0, 0
	for _, call := range graph.Calls {
		switch call.Display {
		case "helper.format":
			if call.TargetID == "" || call.Confidence != "import-resolved" || len(call.CandidateTargetIDs) != 0 {
				t.Fatalf("exact Dart import call=%+v", call)
			}
			resolved++
		case "remote.format":
			if call.TargetID != "" {
				t.Fatalf("Dart package import guessed a target: %+v", call)
			}
			unresolved++
		}
	}
	if resolved != 1 || unresolved != 1 {
		t.Fatalf("Dart calls=%+v", graph.Calls)
	}
}

func TestDartUnprefixedImportsDoNotExposeUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	sources := []TextSource{
		{Path: filepath.Join(root, "lib/main.dart"), Text: "import 'src/helper.dart';\nvoid run() { format(); hidden(); }\n"},
		{Path: filepath.Join(root, "lib/src/helper.dart"), Text: "void format() {}\n"},
		{Path: filepath.Join(root, "lib/other.dart"), Text: "void hidden() {}\n"},
	}
	analysis, stats := NewGraphEngine(BuildOptions{DisableCache: true}).BuildTextSources(sources)
	graph := analysis.Graph()
	if stats.Parsed != 3 || stats.Failed != 0 {
		t.Fatalf("Dart source stats=%+v", stats)
	}
	seen := map[string]bool{}
	for _, call := range graph.Calls {
		if call.Name == "format" && call.TargetID != "" {
			seen["resolved"] = true
		}
		if call.Name == "hidden" && call.TargetID == "" {
			seen["unrelated-unresolved"] = true
		}
	}
	if !seen["resolved"] || !seen["unrelated-unresolved"] {
		t.Fatalf("Dart unprefixed call targets=%+v", graph.Calls)
	}
}
