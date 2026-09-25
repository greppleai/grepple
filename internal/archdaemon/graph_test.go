package archdaemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/analysis"
)

func TestFocusedGraphAndResolveDaemonVariants(t *testing.T) {
	_ = daemonTestRoot(t)
	const source = "package p\nfunc Target() {}\nfunc Caller() { Target() }\n"
	if err := os.WriteFile("main.go", []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"main.go"}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	var details descriptor
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if current, ok := readDescriptor(); ok && daemonAlive(current) {
			details = current
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if details.Token == "" {
		t.Fatal("daemon did not start")
	}
	for _, route := range []string{"/graph", "/resolve"} {
		request, err := http.NewRequest(http.MethodPost, "http://"+details.Address+route, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		unauthorized, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = unauthorized.Body.Close()
		if unauthorized.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated status=%d", route, unauthorized.StatusCode)
		}
	}
	query := analysis.GraphQuery{Direction: "callers", Depth: 1, Symbol: "Target"}
	other := analysis.GraphQuery{Direction: "impact", Depth: 2, Symbol: "Target"}
	selection := ResolveSelection{Symbol: "Target"}
	if _, hit := QueryGraph(paths, 0, query); hit {
		t.Fatal("unpublished graph was a hit")
	}
	sources := analysis.ReadSources(paths)
	graphKey, ok := KeyGraph(paths, 0, sources, query)
	if !ok {
		t.Fatal("graph key unavailable")
	}
	otherKey, ok := KeyGraph(paths, 0, sources, other)
	if !ok || otherKey == graphKey {
		t.Fatal("graph selectors shared a key")
	}
	resolveKey, ok := KeyResolve(paths, 0, sources, selection)
	if !ok || resolveKey == graphKey {
		t.Fatal("resolve selector shared a graph key")
	}
	universe, err := analysis.NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	graph, err := analysis.BuildGraph(universe, &query)
	if err != nil {
		t.Fatal(err)
	}
	if !StoreGraph(paths, 0, graphKey, query, graph) {
		t.Fatal("graph store failed")
	}
	cached, hit := QueryGraph(paths, 0, query)
	if !hit {
		t.Fatal("graph store was not visible")
	}
	want, _ := json.Marshal(graph)
	got, _ := json.Marshal(cached)
	if !bytes.Equal(want, got) {
		t.Fatal("graph cache changed canonical report")
	}
	if _, hit := QueryGraph(paths, 0, other); hit {
		t.Fatal("query variants collided")
	}
	if StoreGraph(paths, 0, graphKey, other, graph) {
		t.Fatal("mismatched selection key accepted")
	}
	projection := ResolveProjection{Schema: ResolveProjectionSchema, Sources: graph.Sources, Declarations: graph.Declarations}
	if !StoreResolve(paths, 0, resolveKey, selection, projection) {
		t.Fatal("resolve store failed")
	}
	if cached, hit := QueryResolve(paths, 0, selection); !hit || cached.Schema != ResolveProjectionSchema || len(cached.Declarations) != len(graph.Declarations) {
		t.Fatalf("resolve hit=%v projection=%+v", hit, cached)
	}
	if _, hit := QueryResolve(paths, 0, ResolveSelection{Symbol: "Caller"}); hit {
		t.Fatal("resolve selectors collided")
	}
	if err := os.WriteFile("main.go", []byte(source+"func New() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, hit := QueryGraph(paths, 0, query); hit {
		t.Fatal("source change reused graph")
	}
	if _, hit := QueryResolve(paths, 0, selection); hit {
		t.Fatal("source change reused resolve projection")
	}
	if StoreGraph(paths, 0, graphKey, query, graph) || StoreResolve(paths, 0, resolveKey, selection, projection) {
		t.Fatal("stale report accepted")
	}
}

func TestReportCacheRetainsMultipleVariantsPerRoot(t *testing.T) {
	cache := newReportCache()
	root := t.TempDir()
	for index := 0; index < maxCachedVariantsPerRoot; index++ {
		key := string(rune('a' + index))
		if err := cache.putVariant(root, graphVariant, key, index, 100); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := cache.getVariant(root, graphVariant, "a"); !ok {
		t.Fatal("initial graph variant missing")
	}
	if err := cache.putVariant(root, resolveVariant, "new", "projection", 100); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.getVariant(root, graphVariant, "b"); ok {
		t.Fatal("least-recent variant was not evicted")
	}
	if _, ok := cache.getVariant(root, graphVariant, "a"); !ok {
		t.Fatal("recent graph variant was evicted")
	}
	if _, ok := cache.getVariant(root, resolveVariant, "new"); !ok {
		t.Fatal("resolve variant was evicted")
	}
	if cache.bytes != maxCachedVariantsPerRoot*100 {
		t.Fatalf("cache bytes=%d", cache.bytes)
	}
}

func TestReportCacheEvictsLeastRecentlyUsedRootNotOldestVariant(t *testing.T) {
	cache := newReportCache()
	roots := make([]string, maxCachedRoots+1)
	for index := range roots {
		roots[index] = t.TempDir()
	}
	if err := cache.putVariant(roots[0], graphVariant, "old", 0, 100); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < maxCachedRoots; index++ {
		if err := cache.putVariant(roots[index], graphVariant, "only", index, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err := cache.putVariant(roots[0], resolveVariant, "recent", 0, 100); err != nil {
		t.Fatal(err)
	}
	if err := cache.putVariant(roots[maxCachedRoots], graphVariant, "only", 0, 100); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.getVariant(roots[1], graphVariant, "only"); ok {
		t.Fatal("least recently touched root should be evicted")
	}
	for _, entry := range []struct{ root, kind, key string }{
		{roots[0], graphVariant, "old"},
		{roots[0], resolveVariant, "recent"},
		{roots[maxCachedRoots], graphVariant, "only"},
	} {
		if _, ok := cache.getVariant(entry.root, entry.kind, entry.key); !ok {
			t.Fatalf("recent root lost variant %+v", entry)
		}
	}
}
