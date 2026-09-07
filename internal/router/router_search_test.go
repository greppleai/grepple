package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grepple/internal/api"
	"grepple/internal/search"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRouteSearchReturnsPartialResultsWhenSomeBackendsFail(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{{Path: "owner/repo/file.go"}}})
	}))
	defer healthy.Close()
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failed.Close()

	query := "deploy"
	// One shard up, one down: the search should still return the healthy shard's
	// results and report the failed shard rather than failing entirely.
	resp, err := routeSearch(routerOptions{
		backends: []string{healthy.URL, failed.URL},
		timeout:  time.Second,
	}, api.SearchRequest{Query: &query})
	if err != nil {
		t.Fatalf("partial search should succeed, got: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Path != "owner/repo/file.go" {
		t.Fatalf("expected the healthy shard's result, got %#v", resp.Results)
	}
	if len(resp.ShardErrors) != 1 {
		t.Fatalf("expected 1 shard error, got %v", resp.ShardErrors)
	}
}

func TestRouteSearchFailsOnlyWhenAllBackendsFail(t *testing.T) {
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failed.Close()
	query := "deploy"
	_, err := routeSearch(routerOptions{
		backends: []string{failed.URL},
		timeout:  time.Second,
	}, api.SearchRequest{Query: &query})
	if err == nil {
		t.Fatal("expected error when every shard fails")
	}
}

func TestRouterReportsUnavailableBackend(t *testing.T) {
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failed.Close()
	options := routerOptions{backends: []string{failed.URL}, timeout: time.Second, ring: newRing([]string{failed.URL}), concurrency: 1}
	state := &routerState{o: options, q: newQueue(options)}
	app := state.handler()

	// Readiness reports the router's own state only: it stays ready (200) even
	// when a shard is unreachable.
	health, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d want 200 (own state)", health.StatusCode)
	}

	// Shard health surfaces on the informational /backends endpoint instead.
	backends, err := app.Test(httptest.NewRequest(http.MethodGet, "/backends", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Body.Close()
	if backends.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("backends status=%d want 503 when a shard is down", backends.StatusCode)
	}

	search, err := app.Test(httptest.NewRequest(http.MethodPost, "/public/search", strings.NewReader(`{"query":"deploy"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer search.Body.Close()
	if search.StatusCode != http.StatusBadGateway {
		t.Fatalf("search status=%d", search.StatusCode)
	}
}

// TestRouteSearchPushesPagingWindowToShards verifies the router asks each shard
// for only the top (skip+limit) ranked files (with skip=0 and no raw MaxFiles)
// so paging is applied during the shard's initial filtering, not after transfer.
func TestRouteSearchPushesPagingWindowToShards(t *testing.T) {
	check := func(t *testing.T, req api.SearchRequest, wantLimit int, wantLimitSet bool) {
		t.Helper()
		var got api.SearchRequest
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{}})
		}))
		defer backend.Close()
		if _, err := routeSearch(routerOptions{backends: []string{backend.URL}, timeout: time.Second}, req); err != nil {
			t.Fatalf("routeSearch: %v", err)
		}
		if got.Skip == nil || *got.Skip != 0 {
			t.Fatalf("shard Skip = %v, want 0", got.Skip)
		}
		if got.MaxFiles != nil {
			t.Fatalf("shard MaxFiles = %v, want nil (folded into Limit)", *got.MaxFiles)
		}
		if wantLimitSet {
			if got.Limit == nil || *got.Limit != wantLimit {
				t.Fatalf("shard Limit = %v, want %d", got.Limit, wantLimit)
			}
		} else if got.Limit != nil {
			t.Fatalf("shard Limit = %v, want nil (unbounded)", *got.Limit)
		}
	}
	query := "deploy"
	p := func(n int) *int { return &n }
	t.Run("skip+limit", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query, Skip: p(2), Limit: p(3)}, 5, true)
	})
	t.Run("skip+maxfiles folds into limit", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query, Skip: p(2), MaxFiles: p(4)}, 6, true)
	})
	t.Run("unset limit defaults to the server page default", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query}, search.DefaultPageLimit, true)
	})
	t.Run("explicit zero clamps to the server page cap", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query, Limit: p(0)}, search.MaxPageLimit, true)
	})
	t.Run("oversized limit clamps to the server page cap", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query, Limit: p(500)}, search.MaxPageLimit, true)
	})
	t.Run("deep skip composes with the capped limit", func(t *testing.T) {
		check(t, api.SearchRequest{Query: &query, Skip: p(250), Limit: p(500)}, 250+search.MaxPageLimit, true)
	})
}

func TestRouteSearchCountByRepoSumsAcrossShardsAndDropsLimit(t *testing.T) {
	var gotCountByRepo bool
	var gotLimit *int
	shardA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.SearchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotCountByRepo = req.CountByRepo
		gotLimit = req.Limit
		_ = json.NewEncoder(w).Encode(api.SearchResponse{RepoCounts: []api.RepoCount{
			{Repo: "owner/alpha", Files: 2, Matches: 5},
		}})
	}))
	defer shardA.Close()
	shardB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{RepoCounts: []api.RepoCount{
			{Repo: "owner/beta", Files: 1, Matches: 9},
			{Repo: "owner/alpha", Files: 1, Matches: 1},
		}})
	}))
	defer shardB.Close()

	query := "ping"
	limit := 20
	resp, err := routeSearch(routerOptions{
		backends: []string{shardA.URL, shardB.URL},
		timeout:  time.Second,
	}, api.SearchRequest{Query: &query, CountByRepo: true, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if !gotCountByRepo {
		t.Fatal("shard request should carry CountByRepo=true")
	}
	if gotLimit != nil {
		t.Fatalf("counts must not push a paging limit to shards, got %v", *gotLimit)
	}
	if len(resp.RepoCounts) != 2 {
		t.Fatalf("expected 2 repos, got %#v", resp.RepoCounts)
	}
	// beta (9) sorts before the summed alpha (5+1=6).
	if resp.RepoCounts[0].Repo != "owner/beta" || resp.RepoCounts[0].Matches != 9 {
		t.Fatalf("unexpected first row: %#v", resp.RepoCounts[0])
	}
	if resp.RepoCounts[1].Repo != "owner/alpha" || resp.RepoCounts[1].Files != 3 || resp.RepoCounts[1].Matches != 6 {
		t.Fatalf("alpha should sum across shards to files=3 matches=6, got %#v", resp.RepoCounts[1])
	}
}

func TestRouteSearchLogsShardTimings(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{{Path: "owner/repo/a.go"}}})
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()

	query := "needle"
	if _, err := routeSearch(routerOptions{
		backends: []string{good.URL, bad.URL},
		timeout:  time.Second,
		logger:   logger,
	}, api.SearchRequest{Query: &query}); err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}

	entries := logs.FilterMessage("search").All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one search log line, got %d", len(entries))
	}
	e := entries[0]
	// A failing shard must escalate the line to Info so it is visible at the
	// production log level, not buried at Debug.
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("expected Info level when a shard fails, got %s", e.Level)
	}
	fields := e.ContextMap()
	if fields["failed"] != int64(1) {
		t.Fatalf("expected failed=1, got %v", fields["failed"])
	}
	if fields["backends"] != int64(2) {
		t.Fatalf("expected backends=2, got %v", fields["backends"])
	}
	if _, ok := fields["totalMs"]; !ok {
		t.Fatal("expected a totalMs field")
	}
	if _, ok := fields["shards"]; !ok {
		t.Fatal("expected a per-shard timing breakdown field")
	}
}

func TestRouteSearchOrdersByPathAndPropagatesTruncated(t *testing.T) {
	// shardA returns a lexically-later file; shardB returns earlier files and
	// reports truncation. The merged result must be ordered purely by path (no
	// relevance ranking) and carry Truncated=true.
	shardA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{
			{Path: "owner/z-repo/zzz.go"},
		}})
	}))
	defer shardA.Close()
	shardB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{
			Results: []api.FileResult{
				{Path: "owner/a-repo/mmm.go"},
				{Path: "owner/a-repo/aaa.go"},
			},
			Truncated: true,
		})
	}))
	defer shardB.Close()

	query := "x"
	resp, err := routeSearch(routerOptions{
		backends: []string{shardA.URL, shardB.URL},
		timeout:  time.Second,
	}, api.SearchRequest{Query: &query})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated {
		t.Fatal("expected Truncated to propagate when any shard truncated")
	}
	got := []string{}
	for _, r := range resp.Results {
		got = append(got, r.Path)
	}
	want := []string{"owner/a-repo/aaa.go", "owner/a-repo/mmm.go", "owner/z-repo/zzz.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected deterministic path order %v, got %v", want, got)
	}
}

func TestRouteSearchSendsRequestIDHeaderAndLogsIt(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	var gotHeader string
	shard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Request-Id")
		_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{{Path: "owner/repo/a.go"}}})
	}))
	defer shard.Close()

	query := "x"
	if _, err := routeSearch(routerOptions{
		backends: []string{shard.URL},
		timeout:  time.Second,
		logger:   logger,
	}, api.SearchRequest{Query: &query}); err != nil {
		t.Fatal(err)
	}

	if gotHeader == "" {
		t.Fatal("shard request must carry an X-Request-Id header")
	}
	entries := logs.FilterMessage("search").All()
	if len(entries) != 1 {
		t.Fatalf("expected one search log line, got %d", len(entries))
	}
	if entries[0].ContextMap()["reqId"] != gotHeader {
		t.Fatalf("router log reqId %v must match the header sent to the shard %q",
			entries[0].ContextMap()["reqId"], gotHeader)
	}
}
