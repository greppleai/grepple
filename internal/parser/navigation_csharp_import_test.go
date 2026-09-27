package parser

import "testing"

func TestCSharpNavigationImportsFileScopedNamespaceAndAliasFacts(t *testing.T) {
	content := `global using Common.Shared;
using Lib.Api;
using HelperAlias = Lib.Api.Helper;
using ApiAlias = Lib.Api;
using static Lib.Tools.Actions;
namespace App.Service;
public class Service {
    void Use() { HelperAlias.Work(); Run(); }
}
`
	graph := BuildNavigationGraph(content, "csharp", "Service.cs")
	assertNavigationImport(t, graph, "*", "Common.Shared", "*", 1)
	assertNavigationImport(t, graph, "*", "Lib.Api", "*", 2)
	assertNavigationImport(t, graph, "HelperAlias", "Lib.Api.Helper", "Helper", 3)
	assertNavigationImport(t, graph, "ApiAlias", "Lib.Api", "Api", 4)
	assertNavigationImport(t, graph, "*", "Lib.Tools.Actions", "", 5)
	assertNavigationPackage(t, graph, "App.Service")
	assertNavigationExport(t, graph, "Service", "App.Service")
	for _, call := range graph.Calls {
		if call.Display == "HelperAlias.Work" && call.ImportPath != "" {
			t.Fatalf("namespace-or-type alias was promoted without repository evidence: %#v", call)
		}
	}
}

func TestCSharpNavigationBlockNamespaceIncludesNamespaceUsings(t *testing.T) {
	content := `using Root.Shared;
namespace App.Service {
    using Local.Shared;
    public class Service { void Use() {} }
}
`
	graph := BuildNavigationGraph(content, "csharp", "Service.cs")
	assertNavigationImport(t, graph, "*", "Root.Shared", "*", 1)
	assertNavigationImport(t, graph, "*", "Local.Shared", "*", 3)
	assertNavigationPackage(t, graph, "App.Service")
	assertNavigationExport(t, graph, "Service", "App.Service")
}

func TestCSharpNavigationMultipleNamespacesRemainUnscoped(t *testing.T) {
	content := `using Root.Shared;
namespace First { public class One {} }
namespace Second { using Local.Shared; public class Two {} }
`
	graph := BuildNavigationGraph(content, "csharp", "Mixed.cs")
	assertNavigationImport(t, graph, "*", "Root.Shared", "*", 1)
	if len(graph.Exports) != 0 {
		t.Fatalf("ambiguous namespace exports=%#v", graph.Exports)
	}
	for _, declaration := range graph.Declarations {
		if declaration.Package != "" {
			t.Fatalf("ambiguous namespace package=%q declarations=%#v", declaration.Package, graph.Declarations)
		}
	}
	if len(graph.Imports) != 1 {
		t.Fatalf("namespace-local import escaped its scope: %#v", graph.Imports)
	}
}

func TestCSharpNavigationExportsOmitNonPublicTopLevelDeclarations(t *testing.T) {
	graph := BuildNavigationGraph("namespace App; internal class Hidden {}", "csharp", "Hidden.cs")
	if len(graph.Exports) != 0 {
		t.Fatalf("exports=%#v", graph.Exports)
	}
}

func TestCSharpNavigationMultilineStaticUsing(t *testing.T) {
	graph := BuildNavigationGraph("using static\n    Lib.Tools.Actions;\n", "csharp", "GlobalUsings.cs")
	assertNavigationImport(t, graph, "*", "Lib.Tools.Actions", "", 1)
}

func TestCachedCSharpNavigationFactsInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "using Alias = Lib.Api.Helper;\nnamespace App;\npublic class Service { void Use() { Alias.Work(); } }\n"
	cold, _, coldHit, err := CachedNavigationGraph(content, "csharp", "first/Service.cs")
	if err != nil {
		t.Fatal(err)
	}
	warm, _, warmHit, err := CachedNavigationGraph(content, "csharp", "moved/Service.cs")
	if err != nil {
		t.Fatal(err)
	}
	if coldHit || !warmHit || len(cold.Imports) != 1 || len(warm.Imports) != 1 || len(cold.Exports) != 1 || len(warm.Exports) != 1 {
		t.Fatalf("coldHit=%v warmHit=%v cold=%#v warm=%#v", coldHit, warmHit, cold, warm)
	}
	if cold.Imports[0].Path != "first/Service.cs" || warm.Imports[0].Path != "moved/Service.cs" || cold.Exports[0].Path != "first/Service.cs" || warm.Exports[0].Path != "moved/Service.cs" {
		t.Fatalf("cold imports/exports=%#v/%#v warm=%#v/%#v", cold.Imports, cold.Exports, warm.Imports, warm.Exports)
	}
}
