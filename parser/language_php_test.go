package parser

import (
	"os"
	"testing"
)

func TestPHPNavigationAndOutline(t *testing.T) {
	bytes, err := os.ReadFile("../testdata/php/sample.php")
	if err != nil {
		t.Fatal(err)
	}
	content := string(bytes)
	outline := OutlineFile("../testdata/php/sample.php", content)
	if outline.Language != "php" || len(outline.Symbols) != 5 {
		t.Fatalf("PHP outline: %#v", outline)
	}
	if outline.Symbols[2].Name != "Greeter" || len(outline.Symbols[2].Children) != 3 || outline.Symbols[2].Children[0].Name != "prefix" {
		t.Fatalf("PHP class outline: %#v", outline.Symbols[2])
	}
	graph := BuildNavigationGraph(content, "php", "sample.php")
	if len(graph.Declarations) != 5 || len(graph.TypeDeclarations) != 2 || len(graph.Imports) != 1 {
		t.Fatalf("PHP navigation facts: %#v", graph)
	}
	if graph.Imports[0].Alias != "Format" || graph.Imports[0].ImportPath != `App\Support\Formatter` {
		t.Fatalf("PHP use alias: %#v", graph.Imports)
	}
	if len(graph.Fields) != 1 || graph.Fields[0].Name != "prefix" || graph.Fields[0].Visibility != NavigationVisibilityNonPublic {
		t.Fatalf("PHP typed field: %#v", graph.Fields)
	}
	if len(graph.Calls) != 2 || len(graph.TypeUsages) == 0 || len(graph.MemberAccesses) != 2 {
		t.Fatalf("PHP calls, types, members: %#v", graph)
	}
}

func TestPHPDistinctNamespacesDoNotMergeImportBindings(t *testing.T) {
	content := `<?php
namespace First {
 use Vendor\First\Service as Service;
 class One { public function go() { Service::run(); } }
}
namespace Second {
 use Vendor\Second\Service as Service;
 class Two { public function go() { Service::run(); } }
}
`
	graph := BuildNavigationGraph(content, "php", "two.php")
	if len(graph.Exports) != 2 || graph.Exports[0].ImportPath != "First" || graph.Exports[1].ImportPath != "Second" {
		t.Fatalf("namespace exports=%#v", graph.Exports)
	}
	if len(graph.Imports) != 0 || len(graph.Calls) != 2 {
		t.Fatalf("ambiguous file-wide imports must not be promoted: imports=%#v calls=%#v", graph.Imports, graph.Calls)
	}
	for _, call := range graph.Calls {
		if call.ImportPath != "" {
			t.Fatalf("false cross-namespace import on call: %#v", call)
		}
	}
}
