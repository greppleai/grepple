package parser

import "testing"

func TestJavaNavigationImportsPackageAndCallContext(t *testing.T) {
	content := `package app.service;
import lib.api.Helper;
import static lib.tools.Actions.run;
import lib.models.*;
public class Service {
    void use() {
        Helper.work();
        run();
    }
}
`
	graph := BuildNavigationGraph(content, "java", "src/Service.java")
	assertNavigationImport(t, graph, "Helper", "lib.api.Helper", "Helper", 2)
	assertNavigationImport(t, graph, "run", "lib.tools.Actions", "run", 3)
	assertNavigationImport(t, graph, "*", "lib.models", "*", 4)
	assertNavigationPackage(t, graph, "app.service")
	assertNavigationExport(t, graph, "Service", "app.service")
	assertNavigationCallImport(t, graph, "Helper.work", "lib.api.Helper", "work")
	assertNavigationCallImport(t, graph, "run", "lib.tools.Actions", "run")
}

func TestKotlinNavigationImportsAliasesPackageAndCallContext(t *testing.T) {
	content := `package app.service
import lib.api.Helper as Renamed
import lib.tools.run
import lib.models.*
public class Service {
    fun use() {
        Renamed.work()
        run()
    }
}
`
	graph := BuildNavigationGraph(content, "kotlin", "src/Service.kt")
	assertNavigationImport(t, graph, "Renamed", "lib.api.Helper", "Helper", 2)
	assertNavigationImport(t, graph, "run", "lib.tools.run", "run", 3)
	assertNavigationImport(t, graph, "*", "lib.models", "*", 4)
	assertNavigationPackage(t, graph, "app.service")
	assertNavigationExport(t, graph, "Service", "app.service")
	assertNavigationCallImport(t, graph, "Renamed.work", "lib.api.Helper", "work")
	assertNavigationCallImport(t, graph, "run", "lib.tools.run", "run")
}

func TestJVMNavigationExportsOmitNonPublicTopLevelDeclarations(t *testing.T) {
	java := BuildNavigationGraph("package app; class Hidden {}", "java", "Hidden.java")
	if len(java.Exports) != 0 {
		t.Fatalf("Java exports=%#v", java.Exports)
	}
	kotlin := BuildNavigationGraph("package app\nprivate fun hidden() {}\ninternal class Internal", "kotlin", "Hidden.kt")
	if len(kotlin.Exports) != 0 {
		t.Fatalf("Kotlin exports=%#v", kotlin.Exports)
	}
}

func TestCachedJVMNavigationFactsInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "package app;\nimport lib.api.Helper;\npublic class Service { void use() { Helper.work(); } }\n"
	cold, _, coldHit, err := CachedNavigationGraph(content, "java", "first/Service.java")
	if err != nil {
		t.Fatal(err)
	}
	warm, _, warmHit, err := CachedNavigationGraph(content, "java", "moved/Service.java")
	if err != nil {
		t.Fatal(err)
	}
	if coldHit || !warmHit || len(cold.Imports) != 1 || len(warm.Imports) != 1 || len(cold.Exports) != 1 || len(warm.Exports) != 1 {
		t.Fatalf("coldHit=%v warmHit=%v cold=%#v warm=%#v", coldHit, warmHit, cold, warm)
	}
	if cold.Imports[0].Path != "first/Service.java" || warm.Imports[0].Path != "moved/Service.java" || cold.Exports[0].Path != "first/Service.java" || warm.Exports[0].Path != "moved/Service.java" {
		t.Fatalf("cold imports/exports=%#v/%#v warm=%#v/%#v", cold.Imports, cold.Exports, warm.Imports, warm.Exports)
	}
}

func assertNavigationImport(t *testing.T, graph NavigationGraph, alias, importPath, imported string, line int) {
	t.Helper()
	for _, item := range graph.Imports {
		if item.Alias == alias && item.ImportPath == importPath && item.Imported == imported && item.Line == line {
			return
		}
	}
	t.Fatalf("missing import alias=%q path=%q imported=%q line=%d: %#v", alias, importPath, imported, line, graph.Imports)
}

func assertNavigationExport(t *testing.T, graph NavigationGraph, name, packageName string) {
	t.Helper()
	for _, item := range graph.Exports {
		if item.Name == name && item.ImportPath == packageName {
			return
		}
	}
	t.Fatalf("missing export name=%q package=%q: %#v", name, packageName, graph.Exports)
}

func assertNavigationPackage(t *testing.T, graph NavigationGraph, expected string) {
	t.Helper()
	if len(graph.Declarations) == 0 {
		t.Fatalf("declarations=%#v", graph.Declarations)
	}
	for _, declaration := range graph.Declarations {
		if declaration.Package != expected {
			t.Fatalf("declaration package=%q, want %q: %#v", declaration.Package, expected, graph.Declarations)
		}
	}
}

func assertNavigationCallImport(t *testing.T, graph NavigationGraph, display, importPath, resolvedName string) {
	t.Helper()
	for _, call := range graph.Calls {
		if call.Display == display && call.ImportPath == importPath && call.ResolvedName == resolvedName {
			return
		}
	}
	t.Fatalf("missing call display=%q path=%q resolved=%q: %#v", display, importPath, resolvedName, graph.Calls)
}
