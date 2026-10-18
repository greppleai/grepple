package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/daemon"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/parser"
)

func TestGraphMetadataColdWarmStoredAndDaemonHitMiss(t *testing.T) {
	root := graphMetadataFixture(t)
	cache := filepath.Join(t.TempDir(), "private-cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_CACHE_DIR", cache)
	t.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(cache, "navigation"))
	selector, err := compileGraphMetadataSelector([]string{"implementation"}, []string{"payments"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{"."}, selector)
	if err != nil || len(paths) != 1 {
		t.Fatalf("selected paths=%v err=%v", paths, err)
	}
	query := analysis.GraphQuery{Direction: "callees", Depth: 1, Symbol: "Root"}
	for run := 0; run < 2; run++ {
		result := graphMetadataJSON(t, "callees", "--symbol", "Root", "--purpose", "implementation", "--domain-area", "payments", "--json", ".")
		encoded, err := json.Marshal(result)
		if err != nil || !reflect.DeepEqual(graphOutputDeclarationNames(result), []string{"Root"}) || result.Sources.Parsed != 1 || strings.Contains(string(encoded), `"b.go"`) {
			t.Fatalf("stored run=%d leaked excluded source: %+v err=%v", run, result, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- daemon.Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Errorf("daemon shutdown: %v", err)
		}
	}()
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, ok := daemon.KeyGraph(paths, 0, analysis.ReadSources(paths), query); ok {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("live daemon never became ready")
	}
	if _, hit := daemon.QueryGraph(paths, 0, query); hit {
		t.Fatal("expected cold query miss")
	}
	var first, second bytes.Buffer
	values := &QueryArgs{Symbol: "Root", Depth: 1, JSON: true, Purposes: []string{"implementation"}, DomainAreas: []string{"payments"}, Paths: []string{"."}}
	for _, buffer := range []*bytes.Buffer{&first, &second} {
		if err := executeQueryWithDaemon(cliruntime.Environment{Output: buffer}, "callees", values, true); err != nil {
			t.Fatal(err)
		}
	}
	if first.String() != second.String() {
		t.Fatalf("daemon miss/hit changed JSON output\nfirst=%s\nsecond=%s", first.String(), second.String())
	}
	cached, hit := daemon.QueryGraph(paths, 0, query)
	if !hit || cached.Sources.Parsed != 1 || len(cached.Declarations) != 1 {
		t.Fatalf("daemon graph hit=%v report=%+v", hit, cached)
	}
	encoded, err := json.Marshal(cached)
	if err != nil || strings.Contains(string(encoded), `"b.go"`) {
		t.Fatalf("cached report leaked excluded source: %s err=%v", encoded, err)
	}
	selection := daemon.ResolveSelection{Symbol: "Root"}
	if _, hit := daemon.QueryResolve(paths, 0, selection); hit {
		t.Fatal("expected cold resolve miss")
	}
	var output bytes.Buffer
	resolve := &ResolveArgs{Symbol: "Root", JSON: true, Purposes: []string{"implementation"}, DomainAreas: []string{"payments"}, Paths: []string{"."}}
	if err := executeResolveWithDaemon(cliruntime.Environment{Output: &output}, resolve, true); err != nil {
		t.Fatal(err)
	}
	if _, hit := daemon.QueryResolve(paths, 0, selection); !hit {
		t.Fatal("expected warm resolve hit")
	}
	var warm bytes.Buffer
	if err := executeResolveWithDaemon(cliruntime.Environment{Output: &warm}, resolve, true); err != nil || output.String() != warm.String() {
		t.Fatalf("resolve miss/hit err=%v miss=%s hit=%s", err, output.String(), warm.String())
	}
	document, err := directorymeta.ReadDocumentV1(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := document.Directory["."]
	for i := range entry.Files {
		if entry.Files[i].Name == "a.go" {
			entry.Files[i].Purpose = "testing"
		}
	}
	document.Directory["."] = entry
	if err := directorymeta.WriteDocumentV1(root, document); err != nil {
		t.Fatal(err)
	}
	if err := executeQueryWithDaemon(cliruntime.Environment{}, "callees", values, true); err == nil {
		t.Fatal("out-of-scope declaration leaked from warm daemon or stored cache")
	}
	var filtered bytes.Buffer
	if err := executeQueryWithDaemon(cliruntime.Environment{Output: &filtered}, "callees", &QueryArgs{Symbol: "Work", Depth: 1, JSON: true, Purposes: []string{"testing"}, DomainAreas: []string{"billing"}, Paths: []string{"."}}, true); err != nil {
		t.Fatal(err)
	}
	var result navigationGraphOutput
	if err := json.Unmarshal(filtered.Bytes(), &result); err != nil || !reflect.DeepEqual(graphOutputDeclarationNames(result), []string{"Work"}) || result.Sources.Parsed != 1 {
		t.Fatalf("new selected scope result=%+v err=%v", result, err)
	}
	if strings.Contains(filtered.String(), `"a.go"`) {
		t.Fatalf("warm worker leaked an excluded declaration: %s", filtered.String())
	}
}

func TestGraphMetadataCancellationAndRemoteRefusal(t *testing.T) {
	graphMetadataFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, run := range []func() error{
		func() error {
			return executeQueryWithDaemon(cliruntime.Environment{InvocationScope: ctx}, "callees", &QueryArgs{Symbol: "Root", Depth: 1, Purposes: []string{"implementation"}, Paths: []string{"."}}, true)
		},
		func() error {
			return executeResolveWithDaemon(cliruntime.Environment{InvocationScope: ctx}, &ResolveArgs{Symbol: "Root", Purposes: []string{"implementation"}, Paths: []string{"."}}, true)
		},
	} {
		if err := run(); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled filtered graph: %v", err)
		}
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()
	for _, direction := range []string{"callers", "callees"} {
		err := New(cliruntime.Environment{}).Run([]string{direction, "--symbol", "Root", "--purpose", "implementation", "--repo", "owner/repo", "--server", server.URL})
		if err == nil || !strings.Contains(err.Error(), "local-only") {
			t.Fatalf("%s remote refusal: %v", direction, err)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("filtered remote contacted HTTP server %d times", got)
	}
}
