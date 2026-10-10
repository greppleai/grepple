package analysis

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func mixedLanguageSources() []Source {
	return []Source{
		{Path: "main.go", Content: []byte("package main\nfunc goRoot(){goLeaf()}\nfunc goLeaf(){}\n")},
		{Path: "main.py", Content: []byte("def pyRoot():\n    pyLeaf()\ndef pyLeaf():\n    pass\n")},
		{Path: "main.sh", Content: []byte("shLeaf(){ echo done; }\nshRoot(){ shLeaf; }\n")},
	}
}
func TestLanguageStoredGraphParityAndSelectiveOpen(t *testing.T) {
	cache := t.TempDir()
	t.Setenv(parser.NavigationCacheDirectoryEnv, cache)
	sources := mixedLanguageSources()
	universe, err := NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	for _, selector := range []GraphQuery{{Symbol: "goLeaf"}, {Symbol: "pyLeaf"}, {Symbol: "shLeaf"}, {RootPath: "."}, {Package: "main"}} {
		for _, direction := range []string{"callers", "callees", "dependencies", "dependents", "impact"} {
			request := selector
			request.Direction = direction
			request.Depth = 2
			assertLanguageGraphParity(t, sources, universe, request)
		}
	}
	filter, _ := navigation.NewGraphOperations().NormalizeFilter(navigation.NavigationGraphFilter{})
	key, path := storedGraphKey(sources, 0, filter)
	manifest, err := readLanguageManifest(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Parts) != 3 {
		t.Fatalf("partitions=%d", len(manifest.Parts))
	}
	goPart := languageManifestPartition(manifest, "go")
	if err := os.WriteFile(languagePartitionPath(path, manifest.Generation, goPart), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	request := GraphQuery{Symbol: "pyLeaf", Direction: "callers", Depth: 2}
	assertLanguageGraphParity(t, sources, universe, request)
	unchanged, err := readLanguageManifest(path, key)
	if err != nil || unchanged.Generation != manifest.Generation {
		t.Fatal("Python query opened or rebuilt unrelated Go core")
	}
	assertLanguageGraphParity(t, sources, universe, GraphQuery{Symbol: "goLeaf", Direction: "callers", Depth: 2})
	rebuilt, err := readLanguageManifest(path, key)
	if err != nil || rebuilt.Generation == manifest.Generation {
		t.Fatal("corrupt selected Go core not recovered")
	}
	// Mutating an unrelated language still invalidates the entire coherent generation.
	sources[0].Content = []byte("package main\nfunc goRoot(){}\nfunc goLeaf(){}\n")
	next, _ := storedGraphKey(sources, 0, filter)
	if next == key {
		t.Fatal("cross-language source fence weakened")
	}
}
func assertLanguageGraphParity(t *testing.T, sources []Source, universe *Universe, request GraphQuery) {
	t.Helper()
	want, err := BuildGraph(universe, &request)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := BuildStoredGraph(sources, 0, request)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("language parity changed for %+v", request)
		}
	}
}
func TestLanguageManifestCorruptionAndUnsafeGeneration(t *testing.T) {
	cache := t.TempDir()
	t.Setenv(parser.NavigationCacheDirectoryEnv, cache)
	sources := mixedLanguageSources()
	request := GraphQuery{Symbol: "pyLeaf", Direction: "callers", Depth: 2}
	if _, err := BuildStoredGraph(sources, 0, request); err != nil {
		t.Fatal(err)
	}
	filter, _ := navigation.NewGraphOperations().NormalizeFilter(navigation.NavigationGraphFilter{})
	key, path := storedGraphKey(sources, 0, filter)
	manifest, err := readLanguageManifest(path, key)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Generation = filepath.Join("..", "outside")
	if validateLanguageManifest(manifest, path, key) == nil {
		t.Fatal("escaping generation accepted")
	}
	if err := os.WriteFile(path+".languages.json", []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildStoredGraph(sources, 0, request); err != nil {
		t.Fatal(err)
	}
}
