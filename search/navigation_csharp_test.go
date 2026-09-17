package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestCSharpImportsResolveNamespaceAliasAndStaticOwnerTargets(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "Service.cs")
	helper := filepath.Join(root, "lib", "Helper.cs")
	actions := filepath.Join(root, "lib", "Actions.cs")
	paths := writeCSharpNavigationFiles(t, map[string]string{
		service: `using Lib.Api;
using HelperAlias = Lib.Api.Helper;
using ApiAlias = Lib.Api;
using static Lib.Api.Actions;
namespace App;
public class Service { void Use() { HelperAlias.Work(); ApiAlias.Work(); Run(); } void Typed(Helper value) { value.Load(); } }
`,
		helper:  "namespace Lib.Api;\npublic class Helper { public static void Work() {} public void Load() {} }\n",
		actions: "namespace Lib.Api;\npublic static class Actions { public static void Run() {} }\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertCSharpImportTargets(t, graph.Imports, "Lib.Api", actions, helper)
	assertCSharpImportAliasTargets(t, graph.Imports, "ApiAlias", actions, helper)
	assertCSharpImportTargets(t, graph.Imports, "Lib.Api.Helper", helper)
	assertCSharpImportTargets(t, graph.Imports, "Lib.Api.Actions", actions)
	assertCSharpAliasCallsRemainUnbound(t, graph.Calls, service, "HelperAlias.Work", "ApiAlias.Work")
	assertCSharpResolvedCall(t, graph.Calls, service, "value.Load")
}

func TestCSharpImportsPreserveDuplicateQualifiedTargetsAsAmbiguous(t *testing.T) {
	root := t.TempDir()
	caller := filepath.Join(root, "app", "Service.cs")
	first := filepath.Join(root, "one", "Helper.cs")
	second := filepath.Join(root, "two", "Helper.cs")
	paths := writeCSharpNavigationFiles(t, map[string]string{
		caller: "using Alias = Lib.Api.Helper; namespace App; public class Service { void Use() { Alias.Work(); } }",
		first:  "namespace Lib.Api; public class Helper { public static void Work() {} }",
		second: "namespace Lib.Api; public class Helper { public static void Work() {} }",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertCSharpImportTargets(t, graph.Imports, "Lib.Api.Helper", first, second)
	for _, call := range graph.Calls {
		if call.Path == caller && call.Display == "Alias.Work" && call.TargetID == "" && len(call.CandidateTargetIDs) == 2 && call.Confidence == "candidate" {
			return
		}
	}
	t.Fatalf("calls=%#v", graph.Calls)
}

func writeCSharpNavigationFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func assertCSharpImportTargets(t *testing.T, imports []parser.NavigationImport, importPath string, expected ...string) {
	t.Helper()
	for _, item := range imports {
		if item.ImportPath == importPath && equalStrings(item.TargetPaths, expected) {
			return
		}
	}
	t.Fatalf("missing import path=%q targets=%#v: %#v", importPath, expected, imports)
}

func assertCSharpResolvedCall(t *testing.T, calls []parser.NavigationCall, source, display string) {
	t.Helper()
	for _, call := range calls {
		strong := call.Confidence == "exact" || call.Confidence == "import-resolved" || call.Confidence == "context-resolved"
		if call.Path == source && call.Display == display && call.TargetID != "" && strong {
			return
		}
	}
	t.Fatalf("missing resolved C# call source=%q display=%q: %#v", source, display, calls)
}

func assertCSharpImportAliasTargets(t *testing.T, imports []parser.NavigationImport, alias string, expected ...string) {
	t.Helper()
	for _, item := range imports {
		if item.Alias == alias && equalStrings(item.TargetPaths, expected) {
			return
		}
	}
	t.Fatalf("missing import alias=%q targets=%#v: %#v", alias, expected, imports)
}

func assertCSharpAliasCallsRemainUnbound(t *testing.T, calls []parser.NavigationCall, source string, displays ...string) {
	t.Helper()
	remaining := make(map[string]bool, len(displays))
	for _, display := range displays {
		remaining[display] = true
	}
	for _, call := range calls {
		if call.Path == source && remaining[call.Display] {
			if call.ImportPath != "" || call.Confidence == "import-resolved" {
				t.Fatalf("uncertain C# alias call was import-resolved: %#v", call)
			}
			delete(remaining, call.Display)
		}
	}
	if len(remaining) != 0 {
		t.Fatalf("missing alias calls=%#v calls=%#v", remaining, calls)
	}
}

func equalStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
