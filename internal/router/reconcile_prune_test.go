package router

import (
	"encoding/json"
	"grepple/internal/api"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// githubMock returns an httptest server that serves the given repos as an org
// repo listing (status 200), or a fixed error status when status >= 300.
func githubMock(t *testing.T, status int, repos []githubRepo) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		if status >= 300 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(repos)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestReconcilePrunesUndesiredRepos verifies reconcile removes an indexed repo
// that is no longer desired (e.g. archived or deleted upstream) while keeping
// the desired one — reconcile is the backstop for missed webhooks.
func TestReconcilePrunesUndesiredRepos(t *testing.T) {
	t.Setenv("GITHUB_API_URL", githubMock(t, 200, []githubRepo{
		{FullName: "testorg/keep", CloneURL: "https://x/keep.git", DefaultBranch: "main"},
	}))
	shard := newFakeShard(true)
	shard.repos["testorg/keep"] = api.RepoInfo{Repo: "testorg/keep"}
	shard.repos["testorg/gone"] = api.RepoInfo{Repo: "testorg/gone"}
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{backends: backends, timeout: time.Second, concurrency: 1, watchOrgs: []string{"testorg"}, ring: newRing(backends)}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()

	pruned, _ := result["pruned"].([]string)
	if len(pruned) != 1 || pruned[0] != "testorg/gone" {
		t.Fatalf("pruned = %v, want [testorg/gone]", result["pruned"])
	}
	if shard.has("testorg/gone") {
		t.Errorf("testorg/gone should have been removed from the shard")
	}
	if !shard.has("testorg/keep") {
		t.Errorf("testorg/keep should have been retained")
	}
}

// TestReconcileSkipsPruneOnDiscoveryError verifies a transient discovery failure
// can never wipe the index: when any source errors, nothing is pruned even
// though an indexed repo is absent from the (incomplete) desired set.
func TestReconcileSkipsPruneOnDiscoveryError(t *testing.T) {
	// Org listing fails (500); a static watched repo keeps desired non-empty so
	// only the error gate — not the empty-desired gate — can suppress pruning.
	t.Setenv("GITHUB_API_URL", githubMock(t, 500, nil))
	shard := newFakeShard(true)
	shard.repos["testorg/keep"] = api.RepoInfo{Repo: "testorg/keep"}
	shard.repos["testorg/gone"] = api.RepoInfo{Repo: "testorg/gone"}
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{
		backends:    backends,
		timeout:     time.Second,
		concurrency: 1,
		watchRepos:  []string{"testorg/keep"},
		watchOrgs:   []string{"testorg"},
		ring:        newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()

	if pruned, _ := result["pruned"].([]string); len(pruned) != 0 {
		t.Fatalf("pruned = %v, want none (discovery errored)", pruned)
	}
	if !shard.has("testorg/gone") || !shard.has("testorg/keep") {
		t.Errorf("no repo should be removed when discovery errors")
	}
}
