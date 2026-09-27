package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestGoWorkspaceModulesRemainFirstPartyWithoutTargetDeclarations(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "go.work", "go 1.25\nuse (\n ./app\n ./lib\n)\nreplace corp.dev/shared => ./lib\n")
	writeRepositoryFixture(t, root, "app/go.mod", "module example.com/app\n")
	writeRepositoryFixture(t, root, "lib/go.mod", "module example.com/lib\n")
	target := writeRepositoryFixture(t, root, "lib/shared.go", "package lib\ntype Shared struct{}\n")
	first := writeRepositoryFixture(t, root, "app/first.go", "package app\nimport lib \"corp.dev/shared\"\nfunc First(value lib.Shared) {}\n")
	second := writeRepositoryFixture(t, root, "app/second.go", "package app\nimport lib \"corp.dev/shared\"\nfunc Second(value lib.Shared) {}\n")

	graph := BuildNavigationGraph([]string{first, second, target})
	if !reflect.DeepEqual(graph.RepositoryRoots, []string{"corp.dev/shared", "example.com/app", "example.com/lib"}) {
		t.Fatalf("repository roots=%#v", graph.RepositoryRoots)
	}
	for _, item := range graph.Imports {
		if len(item.TargetPaths) != 1 || item.TargetPaths[0] != target {
			t.Fatalf("go.work replacement import=%#v", item)
		}
	}
	assertBoundaryImportOrigin(t, graph, "corp.dev/shared", BoundaryTypeOriginFirstParty)
}

func TestNestedGoModulesKeepNearestIdentity(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "go.mod", "module example.com/root\n")
	writeRepositoryFixture(t, root, "plugins/child/go.mod", "module example.com/child\n")
	parent := writeRepositoryFixture(t, root, "main.go", "package root\nfunc Parent() {}\n")
	child := writeRepositoryFixture(t, root, "plugins/child/main.go", "package child\nfunc Child() {}\n")
	graph := BuildNavigationGraph([]string{parent, child})
	if !reflect.DeepEqual(graph.RepositoryRoots, []string{"example.com/child", "example.com/root"}) {
		t.Fatalf("repository roots=%#v", graph.RepositoryRoots)
	}
	modules := map[string]string{}
	for _, declaration := range graph.Declarations {
		modules[declaration.Name] = declaration.ModuleID
	}
	if modules["Parent"] != "example.com/root" || modules["Child"] != "example.com/child" {
		t.Fatalf("module identities=%#v", modules)
	}
}

func TestGoLocalReplacementResolvesImportAndFirstPartyOrigin(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "app/go.mod", "module example.com/app\nrequire github.com/acme/lib v1.0.0\nreplace github.com/acme/lib => ./local-lib\n")
	writeRepositoryFixture(t, root, "app/local-lib/go.mod", "module local.dev/lib\n")
	target := writeRepositoryFixture(t, root, "app/local-lib/client.go", "package lib\ntype Client struct{}\nfunc New() Client { return Client{} }\n")
	first := writeRepositoryFixture(t, root, "app/first.go", "package app\nimport lib \"github.com/acme/lib\"\nfunc First(value lib.Client) { _ = lib.New() }\n")
	second := writeRepositoryFixture(t, root, "app/second.go", "package app\nimport lib \"github.com/acme/lib\"\nfunc Second(value lib.Client) {}\n")

	graph := BuildNavigationGraph([]string{first, second, target})
	if !containsString(graph.RepositoryRoots, "github.com/acme/lib") || len(graph.Imports) != 2 {
		t.Fatalf("roots=%#v imports=%#v", graph.RepositoryRoots, graph.Imports)
	}
	for _, item := range graph.Imports {
		if len(item.TargetPaths) != 1 || item.TargetPaths[0] != target {
			t.Fatalf("replacement import=%#v", item)
		}
	}
	resolvedCall := false
	for _, call := range graph.Calls {
		resolvedCall = resolvedCall || call.Name == "New" && call.TargetID != "" && call.Confidence == "import-resolved"
	}
	if !resolvedCall {
		t.Fatalf("replacement call was not resolved: %#v", graph.Calls)
	}
	assertBoundaryImportOrigin(t, graph, "github.com/acme/lib", BoundaryTypeOriginFirstParty)
	focused := BuildNavigationGraph([]string{first, second})
	if !containsString(focused.RepositoryRoots, "github.com/acme/lib") {
		t.Fatalf("focused replacement roots=%#v", focused.RepositoryRoots)
	}
	assertBoundaryImportOrigin(t, focused, "github.com/acme/lib", BoundaryTypeOriginFirstParty)
}

func TestConflictingLocalReplacementsStayUnresolved(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "go.work", "go 1.25\nuse (\n ./one\n ./two\n ./onelib\n ./twolib\n)\n")
	for _, module := range []string{"one", "two"} {
		writeRepositoryFixture(t, root, module+"/go.mod", "module example.com/"+module+"\nreplace github.com/acme/lib => ../"+module+"lib\n")
		writeRepositoryFixture(t, root, module+"lib/go.mod", "module local.dev/"+module+"lib\n")
		writeRepositoryFixture(t, root, module+"lib/value.go", "package lib\ntype Value struct{}\n")
	}
	first := writeRepositoryFixture(t, root, "one/first.go", "package one\nimport lib \"github.com/acme/lib\"\nfunc First(value lib.Value) {}\n")
	second := writeRepositoryFixture(t, root, "one/second.go", "package one\nimport lib \"github.com/acme/lib\"\nfunc Second(value lib.Value) {}\n")
	paths := []string{first, second, filepath.Join(root, "onelib", "value.go"), filepath.Join(root, "twolib", "value.go"), writeRepositoryFixture(t, root, "two/other.go", "package two\nfunc Other() {}\n")}

	graph := BuildNavigationGraph(paths)
	if containsString(graph.RepositoryRoots, "github.com/acme/lib") {
		t.Fatalf("ambiguous replacement became repository root: %#v", graph.RepositoryRoots)
	}
	for _, item := range graph.Imports {
		if item.ImportPath == "github.com/acme/lib" && len(item.TargetPaths) != 0 {
			t.Fatalf("ambiguous replacement resolved: %#v", item)
		}
	}
	assertBoundaryImportOrigin(t, graph, "github.com/acme/lib", BoundaryTypeOriginThirdParty)
}

func TestUnqualifiedUnknownGoImportRemainsUnresolved(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "go.mod", "module example.com/app\n")
	first := writeRepositoryFixture(t, root, "first.go", "package app\nimport \"shared\"\nfunc First(value shared.Value) {}\n")
	second := writeRepositoryFixture(t, root, "second.go", "package app\nimport \"shared\"\nfunc Second(value shared.Value) {}\n")
	graph := BuildNavigationGraph([]string{first, second})
	assertBoundaryImportOrigin(t, graph, "shared", BoundaryTypeOriginUnresolved)
}

func TestRepositoryIdentityIsColdWarmAndFocusedUniverseInvariant(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	t.Setenv(parser.NavigationCacheDirectoryEnv, cache)
	writeRepositoryFixture(t, root, "go.mod", "module example.com/project\n")
	target := writeRepositoryFixture(t, root, "internal/value.go", "package internal\ntype Value struct{}\n")
	first := writeRepositoryFixture(t, root, "first.go", "package project\nimport \"example.com/project/internal\"\nfunc First(value internal.Value) {}\n")
	second := writeRepositoryFixture(t, root, "second.go", "package project\nimport \"example.com/project/internal\"\nfunc Second(value internal.Value) {}\n")

	cold, _ := BuildNavigationGraphWithOptions([]string{first, second, target}, NavigationBuildOptions{})
	warm, _ := BuildNavigationGraphWithOptions([]string{first, second, target}, NavigationBuildOptions{})
	if navigationGraphBytes(cold) != navigationGraphBytes(warm) {
		t.Fatalf("cold/warm repository graphs differ\ncold=%s\nwarm=%s", navigationGraphBytes(cold), navigationGraphBytes(warm))
	}
	focused := BuildNavigationGraph([]string{first, second})
	assertBoundaryImportOrigin(t, cold, "example.com/project/internal", BoundaryTypeOriginFirstParty)
	assertBoundaryImportOrigin(t, focused, "example.com/project/internal", BoundaryTypeOriginFirstParty)
	if focused.Declarations[0].ModuleID != cold.Declarations[0].ModuleID || focused.Declarations[0].PackageID != cold.Declarations[0].PackageID {
		t.Fatalf("focused identity=%+v full identity=%+v", focused.Declarations[0], cold.Declarations[0])
	}
}

func assertBoundaryImportOrigin(t *testing.T, graph parser.NavigationGraph, importPath string, expected BoundaryTypeOrigin) {
	t.Helper()
	spreads, err := AnalyzeTypeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, spread := range spreads {
		if spread.ImportPath == importPath {
			if spread.Origin != expected {
				t.Fatalf("import %s origin=%s, want %s: %#v", importPath, spread.Origin, expected, spread)
			}
			return
		}
	}
	t.Fatalf("missing spread for %s: %#v", importPath, spreads)
}

func writeRepositoryFixture(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func navigationGraphBytes(graph parser.NavigationGraph) string {
	content, _ := json.Marshal(graph)
	return string(content)
}
