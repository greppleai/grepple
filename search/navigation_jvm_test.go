package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestJavaImportsResolveTypesStaticMembersAndReceiverCalls(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "Service.java")
	helper := filepath.Join(root, "lib", "api", "Helper.java")
	actions := filepath.Join(root, "lib", "tools", "Actions.java")
	paths := writeJVMNavigationFiles(t, map[string]string{
		service: `package app;
import lib.api.Helper;
import static lib.tools.Actions.run;
class Service {
    void use() { Helper.work(); run(); }
}
`,
		helper:  "package lib.api;\npublic class Helper { public static void work() {} public void load() {} }\n",
		actions: "package lib.tools;\npublic class Actions { public static void run() {} }\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertJVMImportTarget(t, graph.Imports, "Helper", helper)
	assertJVMImportTarget(t, graph.Imports, "run", actions)
	assertJVMResolvedCall(t, graph.Calls, service, "Helper.work")
	assertJVMResolvedCall(t, graph.Calls, service, "run")
}

func TestKotlinImportsResolveAliasesAndTopLevelFunctions(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "Service.kt")
	helper := filepath.Join(root, "lib", "api", "Declarations.kt")
	tools := filepath.Join(root, "lib", "tools", "Actions.kt")
	paths := writeJVMNavigationFiles(t, map[string]string{
		service: "package app\nimport lib.api.Helper as Renamed\nimport lib.tools.run\nclass Service { fun use() { Renamed.work(); run() } }\n",
		helper:  "package lib.api\nobject Helper { fun work() {} }\n",
		tools:   "package lib.tools\nfun run() {}\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertJVMImportTarget(t, graph.Imports, "Renamed", helper)
	assertJVMImportTarget(t, graph.Imports, "run", tools)
	assertJVMResolvedCall(t, graph.Calls, service, "Renamed.work")
	assertJVMResolvedCall(t, graph.Calls, service, "run")
}

func TestJVMImportsPreserveDuplicateQualifiedTargetsAsAmbiguous(t *testing.T) {
	root := t.TempDir()
	caller := filepath.Join(root, "app", "Service.java")
	first := filepath.Join(root, "one", "Helper.java")
	second := filepath.Join(root, "two", "Helper.java")
	paths := writeJVMNavigationFiles(t, map[string]string{
		caller: "package app; import lib.api.Helper; class Service { void use() { Helper.work(); } }",
		first:  "package lib.api; public class Helper { static void work() {} }",
		second: "package lib.api; public class Helper { static void work() {} }",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	for _, item := range graph.Imports {
		if item.Alias == "Helper" && len(item.TargetPaths) == 2 && item.TargetPaths[0] == first && item.TargetPaths[1] == second {
			for _, call := range graph.Calls {
				if call.Path == caller && call.Display == "Helper.work" && call.TargetID == "" && len(call.CandidateTargetIDs) == 2 && call.Confidence == "candidate" {
					return
				}
			}
		}
	}
	t.Fatalf("imports=%#v calls=%#v", graph.Imports, graph.Calls)
}

func writeJVMNavigationFiles(t *testing.T, files map[string]string) []string {
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

func assertJVMImportTarget(t *testing.T, imports []parser.NavigationImport, alias, target string) {
	t.Helper()
	for _, item := range imports {
		if item.Alias == alias && len(item.TargetPaths) == 1 && item.TargetPaths[0] == target {
			return
		}
	}
	t.Fatalf("missing import alias=%q target=%q: %#v", alias, target, imports)
}

func assertJVMResolvedCall(t *testing.T, calls []parser.NavigationCall, source, display string) {
	t.Helper()
	for _, call := range calls {
		strong := call.Confidence == "exact" || call.Confidence == "import-resolved" || call.Confidence == "context-resolved"
		if call.Path == source && call.Display == display && call.TargetID != "" && strong {
			return
		}
	}
	t.Fatalf("missing resolved call source=%q display=%q: %#v", source, display, calls)
}
