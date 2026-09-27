package parser

import "testing"

func TestCFamilyNavigationIncludesPreserveDirectSyntax(t *testing.T) {
	for _, language := range []string{"c", "cpp"} {
		graph := BuildNavigationGraph(`#include "local/detail.h"
#include <stdio.h>
#define HEADER "generated.h"
#include HEADER
`, language, "app/service."+language)
		if len(graph.Imports) != 2 {
			t.Fatalf("language=%s imports=%+v", language, graph.Imports)
		}
		assertCFamilyNavigationInclude(t, graph, "local/detail.h", "include-quoted", 1)
		assertCFamilyNavigationInclude(t, graph, "stdio.h", "include-system", 2)
	}
}

func TestCFamilyNavigationIncludesRejectMalformedOrComputedPaths(t *testing.T) {
	graph := BuildNavigationGraph(`#include ""
#include <>
#include HEADER
`, "c", "app/service.c")
	if len(graph.Imports) != 0 {
		t.Fatalf("imports=%+v", graph.Imports)
	}
}

func TestCachedCFamilyNavigationIncludesInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "#include \"detail.h\"\n"
	cold, _, coldHit, err := CachedNavigationGraph(content, "c", "first/service.c")
	if err != nil {
		t.Fatal(err)
	}
	warm, _, warmHit, err := CachedNavigationGraph(content, "c", "moved/service.c")
	if err != nil {
		t.Fatal(err)
	}
	if coldHit || !warmHit || len(cold.Imports) != 1 || len(warm.Imports) != 1 {
		t.Fatalf("coldHit=%v warmHit=%v cold=%#v warm=%#v", coldHit, warmHit, cold.Imports, warm.Imports)
	}
	if cold.Imports[0].Path != "first/service.c" || warm.Imports[0].Path != "moved/service.c" || cold.Imports[0].Kind != warm.Imports[0].Kind || cold.Imports[0].ImportPath != warm.Imports[0].ImportPath {
		t.Fatalf("cold=%#v warm=%#v", cold.Imports[0], warm.Imports[0])
	}
}

func assertCFamilyNavigationInclude(t *testing.T, graph NavigationGraph, importPath, kind string, line int) {
	t.Helper()
	for _, item := range graph.Imports {
		if item.ImportPath == importPath && item.Kind == kind && item.Alias == "*" && item.Imported == "*" && item.Line == line {
			return
		}
	}
	t.Fatalf("include path=%q kind=%q line=%d missing from %+v", importPath, kind, line, graph.Imports)
}
