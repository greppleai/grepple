package navigation

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func resolvedCacheTestDocument(t *testing.T, path, language, content string) DocumentSource {
	t.Helper()
	document, err := parser.ParseDocument(language, content)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(document.Close)
	return DocumentSource{Path: path, Document: document}
}

func resolvedCacheGraphJSON(t *testing.T, graph parser.NavigationGraph) []byte {
	t.Helper()
	content, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestResolvedGraphCacheOptInHitAndDisable(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.go")
	sources := []DocumentSource{resolvedCacheTestDocument(t, path, "go", "package sample\nfunc Target() {}\nfunc Caller() { Target() }\n")}
	cacheDirectory := filepath.Join(t.TempDir(), "navigation")
	t.Setenv(parser.NavigationCacheDirectoryEnv, "")
	uncached, _ := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if _, err := os.Stat(cacheDirectory); !os.IsNotExist(err) {
		t.Fatalf("cache should be opt-in: %v", err)
	}
	t.Setenv(parser.NavigationCacheDirectoryEnv, cacheDirectory)
	cold, coldStats := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if !bytes.Equal(resolvedCacheGraphJSON(t, uncached.Graph()), resolvedCacheGraphJSON(t, cold.Graph())) {
		t.Fatal("cold cache changed graph")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	key, ok := resolvedGraphCacheKey(sources, cwd)
	if !ok {
		t.Fatal("expected cache key")
	}
	if _, hit := readResolvedGraphCache(key); !hit {
		t.Fatal("expected persisted graph")
	}
	marker := cold.Graph()
	marker.RepositoryRoots = []string{"cache-hit"}
	writeResolvedGraphCache(key, marker, false)
	warm, warmStats := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if !bytes.Equal(resolvedCacheGraphJSON(t, warm.Graph()), resolvedCacheGraphJSON(t, marker)) || warmStats != coldStats {
		t.Fatalf("warm cache miss or changed source stats: %+v %+v", coldStats, warmStats)
	}
	disabled, _ := BuildAnalysisFromDocuments(sources, BuildOptions{DisableCache: true})
	if !bytes.Equal(resolvedCacheGraphJSON(t, disabled.Graph()), resolvedCacheGraphJSON(t, uncached.Graph())) {
		t.Fatal("DisableCache did not bypass resolved graph cache")
	}
	changed := []DocumentSource{resolvedCacheTestDocument(t, path, "go", "package sample\nfunc Different() {}\n")}
	changedKey, ok := resolvedGraphCacheKey(changed, cwd)
	if !ok || changedKey == key {
		t.Fatal("source content did not invalidate cache")
	}
	otherPath := []DocumentSource{resolvedCacheTestDocument(t, filepath.Join(directory, "other.go"), "go", sources[0].Document.Source())}
	otherKey, ok := resolvedGraphCacheKey(otherPath, cwd)
	if !ok || otherKey == key {
		t.Fatal("source path did not invalidate cache")
	}
	otherCWD, ok := resolvedGraphCacheKey(sources, directory)
	if !ok || otherCWD == key {
		t.Fatal("working directory did not invalidate cache")
	}
}

func TestResolvedGraphCacheContextInvalidation(t *testing.T) {
	root := t.TempDir()
	t.Setenv(parser.NavigationCacheDirectoryEnv, t.TempDir())
	goSource := resolvedCacheTestDocument(t, filepath.Join(root, "sub", "main.go"), "go", "package sub\nfunc Run() {}\n")
	tsSource := resolvedCacheTestDocument(t, filepath.Join(root, "sub", "main.ts"), "typescript", "export function run() {}\n")
	sources := []DocumentSource{goSource, tsSource}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	key, ok := resolvedGraphCacheKey(sources, cwd)
	if !ok {
		t.Fatal("expected cache key")
	}
	for _, name := range []string{"go.mod", "go.work", "tsconfig.json"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(name+" first"), 0o600); err != nil {
			t.Fatal(err)
		}
		next, ok := resolvedGraphCacheKey(sources, cwd)
		if !ok || next == key {
			t.Fatalf("new %s did not invalidate cache", name)
		}
		key = next
		if err := os.WriteFile(path, []byte(name+" second"), 0o600); err != nil {
			t.Fatal(err)
		}
		next, ok = resolvedGraphCacheKey(sources, cwd)
		if !ok || next == key {
			t.Fatalf("changed %s did not invalidate cache", name)
		}
		key = next
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), bytes.Repeat([]byte("x"), maxResolvedContextFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := resolvedGraphCacheKey(sources, cwd); ok {
		t.Fatal("oversized context should bypass cache")
	}
}

func TestResolvedGraphCacheCorruptionAndAccounting(t *testing.T) {
	t.Setenv(parser.NavigationCacheDirectoryEnv, t.TempDir())
	sources := []DocumentSource{
		resolvedCacheTestDocument(t, "broken.go", "go", "package sample\nfunc Broken( {\n"),
		{Path: "unavailable.go"},
	}
	cold, first := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if first.Attempted != 2 || first.Parsed != 1 || first.Skipped != 1 || first.Recovered != 1 {
		t.Fatalf("cold stats %+v", first)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	key, ok := resolvedGraphCacheKey(sources, cwd)
	if !ok {
		t.Fatal("expected cache key")
	}
	warm, second := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if second != first || !bytes.Equal(resolvedCacheGraphJSON(t, cold.Graph()), resolvedCacheGraphJSON(t, warm.Graph())) {
		t.Fatalf("cached graph or stats changed: %+v %+v", first, second)
	}
	if err := os.WriteFile(resolvedGraphCachePath(key), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, third := BuildAnalysisFromDocuments(sources, BuildOptions{})
	if third != first || !bytes.Equal(resolvedCacheGraphJSON(t, cold.Graph()), resolvedCacheGraphJSON(t, rebuilt.Graph())) {
		t.Fatal("corrupt cache changed results")
	}
	if _, hit := readResolvedGraphCache(key); !hit {
		t.Fatal("corrupt cache was not rebuilt")
	}
}

func TestResolvedGraphCacheConcurrentWriters(t *testing.T) {
	t.Setenv(parser.NavigationCacheDirectoryEnv, t.TempDir())
	const key = "concurrent"
	graph := parser.NavigationGraph{RepositoryRoots: []string{"safe"}}
	want := resolvedCacheGraphJSON(t, graph)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 5; j++ {
				writeResolvedGraphCache(key, graph, false)
				if got, ok := readResolvedGraphCache(key); ok {
					content, err := json.Marshal(got)
					if err != nil || !bytes.Equal(content, want) {
						t.Errorf("concurrent read changed graph: %+v (marshal: %v)", got, err)
					}
				}
			}
		}()
	}
	workers.Wait()
	if got, ok := readResolvedGraphCache(key); !ok || !bytes.Equal(resolvedCacheGraphJSON(t, got), want) {
		t.Fatal("concurrent writers left invalid graph")
	}
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(resolvedGraphCachePath(key)), "*.tmp"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary cache files remain: %v %v", temporary, err)
	}
}
