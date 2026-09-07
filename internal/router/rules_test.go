package router

import (
	"encoding/json"
	"grepple/internal/api"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func qstr(s string) *string { return &s }

func TestRuleRegistryUpsertValidatesAndVersions(t *testing.T) {
	reg := newRuleRegistry(filepath.Join(t.TempDir(), "rules.json"), routerOptions{}, nil)

	if _, err := reg.upsert(api.Rule{ID: "bad", Mode: api.RuleModeCount, Request: api.SearchRequest{}}); err == nil {
		t.Fatal("count rule with no query should be rejected")
	}
	g0 := reg.currentGeneration()
	stored, err := reg.upsert(api.Rule{Name: "Checkout", Request: api.SearchRequest{Query: qstr("actions/checkout")}})
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != "checkout" || stored.CreatedAt == "" {
		t.Fatalf("unexpected stored rule: %#v", stored)
	}
	if reg.currentGeneration() <= g0 {
		t.Fatal("generation should increase on upsert")
	}

	// Re-upsert keeps CreatedAt, bumps generation, updates UpdatedAt.
	g1 := reg.currentGeneration()
	again, _ := reg.upsert(api.Rule{ID: "checkout", Request: api.SearchRequest{Query: qstr("actions/checkout@v4")}})
	if again.CreatedAt != stored.CreatedAt {
		t.Errorf("CreatedAt should be preserved across updates")
	}
	if reg.currentGeneration() <= g1 {
		t.Error("generation should increase on update")
	}
}

func TestRuleRegistryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	reg := newRuleRegistry(path, routerOptions{}, nil)
	if _, err := reg.upsert(api.Rule{ID: "r1", Request: api.SearchRequest{Query: qstr("x")}}); err != nil {
		t.Fatal(err)
	}
	reloaded := newRuleRegistry(path, routerOptions{}, nil)
	if _, ok := reloaded.get("r1"); !ok {
		t.Fatal("rule not recovered from disk")
	}
	if reloaded.currentGeneration() != reg.currentGeneration() {
		t.Errorf("generation not recovered: %d vs %d", reloaded.currentGeneration(), reg.currentGeneration())
	}

	if !reg.remove("r1") {
		t.Fatal("remove should report the rule existed")
	}
	if reg.remove("r1") {
		t.Fatal("second remove should report false")
	}
}

func TestRuleRegistryDistribute(t *testing.T) {
	var mu sync.Mutex
	got := map[string]api.RuleSet{}
	newShard := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && r.URL.Path == "/rules" {
				body, _ := io.ReadAll(r.Body)
				var set api.RuleSet
				_ = json.Unmarshal(body, &set)
				mu.Lock()
				got[r.Host] = set
				mu.Unlock()
			}
			w.WriteHeader(http.StatusOK)
		}))
	}
	a, b := newShard(), newShard()
	defer a.Close()
	defer b.Close()

	reg := newRuleRegistry(filepath.Join(t.TempDir(), "rules.json"),
		routerOptions{backends: []string{a.URL, b.URL}, timeout: 2 * time.Second}, nil)
	if _, err := reg.upsert(api.Rule{ID: "r1", Request: api.SearchRequest{Query: qstr("x")}}); err != nil {
		t.Fatal(err)
	}
	reg.distribute()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected both shards to receive the ruleset, got %d", len(got))
	}
	for host, set := range got {
		if len(set.Rules) != 1 || set.Rules[0].ID != "r1" {
			t.Errorf("shard %s got %#v", host, set)
		}
	}
}

func TestRuleRegistryFetchResultsMerges(t *testing.T) {
	// Two shards each own different repos for the same rule; the router merges.
	shard := func(repos []api.RuleRepoResult) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/results") {
				_ = json.NewEncoder(w).Encode(api.RuleResults{Rule: "r1", Mode: api.RuleModeCount, Repos: repos})
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
	}
	a := shard([]api.RuleRepoResult{{Repo: "o/b", Files: 2, Matches: 3}})
	b := shard([]api.RuleRepoResult{{Repo: "o/a", Files: 1, Matches: 1}})
	defer a.Close()
	defer b.Close()

	reg := newRuleRegistry(filepath.Join(t.TempDir(), "rules.json"),
		routerOptions{backends: []string{a.URL, b.URL}, timeout: 2 * time.Second}, nil)
	if _, err := reg.upsert(api.Rule{ID: "r1", Request: api.SearchRequest{Query: qstr("x")}}); err != nil {
		t.Fatal(err)
	}
	res, err := reg.fetchResults("r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 2 || res.Repos[0].Repo != "o/a" || res.Repos[1].Repo != "o/b" {
		t.Fatalf("expected merged, repo-sorted results, got %#v", res.Repos)
	}

	if _, err := reg.fetchResults("missing"); err == nil {
		t.Error("fetching an unknown rule should error")
	}
}

func TestRuleRegistryRecoverFromBackends(t *testing.T) {
	// A shard holds a higher-generation ruleset; a fresh (empty) router adopts it.
	shard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/rules" {
			_ = json.NewEncoder(w).Encode(api.RuleSet{Generation: 9, Rules: []api.Rule{
				{ID: "recovered", Mode: api.RuleModeCount, Request: api.SearchRequest{Query: qstr("x")}},
			}})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer shard.Close()

	reg := newRuleRegistry(filepath.Join(t.TempDir(), "rules.json"),
		routerOptions{backends: []string{shard.URL}, timeout: 2 * time.Second}, nil)
	reg.recoverFromBackends()
	if reg.currentGeneration() != 9 {
		t.Fatalf("generation = %d, want 9", reg.currentGeneration())
	}
	if _, ok := reg.get("recovered"); !ok {
		t.Fatal("rule not recovered from shard")
	}
}
