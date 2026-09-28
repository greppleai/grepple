package parser

import (
	"os"
	"strings"
	"testing"
)

func TestPHPFixtureNavigationAndOutline(t *testing.T) {
	content, err := os.ReadFile("../testdata/php/sample.php")
	if err != nil {
		t.Fatal(err)
	}
	graph := BuildNavigationGraph(string(content), "php", "sample.php")
	if len(graph.Declarations) < 3 || len(graph.Calls) == 0 || len(graph.TypeDeclarations) == 0 {
		t.Fatalf("incomplete PHP navigation graph: declarations=%v calls=%v types=%v", graph.Declarations, graph.Calls, graph.TypeDeclarations)
	}
	foundNamespace, foundClass, foundImport := false, false, false
	for _, declaration := range graph.Declarations {
		if strings.Contains(declaration.Package, "App.Service") {
			foundNamespace = true
		}
	}
	for _, declaration := range graph.TypeDeclarations {
		if declaration.Kind == "class" && declaration.Name == "Formatter" {
			foundClass = true
		}
	}
	for _, imp := range graph.Imports {
		if imp.ImportPath != "" {
			foundImport = true
		}
	}
	if !foundNamespace || !foundClass || !foundImport || len(graph.Fields) == 0 {
		t.Fatalf("missing namespace/class/import/field: %#v", graph)
	}
	outline := OutlineFile("sample.php", string(content))
	if len(outline.Symbols) == 0 {
		t.Fatal("PHP outline is empty")
	}
}

func TestPHPMixedHTMLAndMalformedSyntax(t *testing.T) {
	for _, source := range []string{"<h1>Welcome</h1>\n<?php function greet() { return 1; } ?>\n<footer>ok</footer>", "<?php function broken(\n"} {
		document, err := ParseDocument("php", source)
		if err != nil {
			t.Fatal(err)
		}
		if !document.Root().Valid() {
			t.Fatal("invalid root")
		}
		document.Close()
	}
}
