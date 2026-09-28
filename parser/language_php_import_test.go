package parser

import "testing"

func TestPHPGroupedAndAliasedNamespaceImports(t *testing.T) {
	graph := BuildNavigationGraph("<?php namespace App; use Foo\\Bar as Named; use Foo\\{Alpha, Beta as Alias};\n", "php", "service.php")
	imports := map[string]string{}
	for _, imp := range graph.Imports {
		imports[imp.Alias] = imp.ImportPath
	}
	for alias, path := range map[string]string{"Named": "Foo.Bar", "Alpha": "Foo.Alpha", "Alias": "Foo.Beta"} {
		if imports[alias] != path {
			t.Fatalf("import %s=%q want %q; all=%#v", alias, imports[alias], path, graph.Imports)
		}
	}
}

func TestPHPExportsRetainNamespaceOwnership(t *testing.T) {
	graph := BuildNavigationGraph("<?php namespace App\\Model { class User {} } namespace App\\Other { class User {} }", "php", "model.php")
	if len(graph.Exports) != 2 || graph.Exports[0].ImportPath != "App.Model" || graph.Exports[1].ImportPath != "App.Other" {
		t.Fatalf("PHP namespace exports=%#v", graph.Exports)
	}
}
