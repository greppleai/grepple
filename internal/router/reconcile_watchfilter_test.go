package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// appAuthServing builds a githubAuth whose single installation is backed by a
// mock GitHub returning the given installation repositories.
func appAuthServing(t *testing.T, repos []map[string]any) (*githubAuth, func()) {
	t.Helper()
	_, key := testKeyPEM(t)
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "inst-token", "expires_at": time.Now().Add(time.Hour)})
		case r.URL.Path == "/installation/repositories":
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": repos})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	it := &installationToken{appID: 1, installationID: 5, org: "testorg", key: key, apiBase: gh.URL, client: gh.Client()}
	auth := &githubAuth{byOrg: map[string]*installationToken{"testorg": it}, installs: []*installationToken{it}}
	return auth, gh.Close
}

func queuedSet(result map[string]any) map[string]bool {
	set := map[string]bool{}
	if q, ok := result["queued"].([]string); ok {
		for _, r := range q {
			set[r] = true
		}
	}
	return set
}

// TestWatchReposAllowlistFiltersAppDiscovery: with watchRepos set, an App repo
// not in the allowlist must NOT be indexed (the fix).
func TestWatchReposAllowlistFiltersAppDiscovery(t *testing.T) {
	auth, closeGH := appAuthServing(t, []map[string]any{
		{"full_name": "testorg/keep", "clone_url": "https://x/keep.git", "default_branch": "main"},
		{"full_name": "testorg/extra", "clone_url": "https://x/extra.git", "default_branch": "main"},
	})
	defer closeGH()

	shard := newFakeShard(true)
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{
		backends: backends, timeout: time.Second, concurrency: 1,
		watchRepos: []string{"testorg/keep"}, auth: auth, ring: newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()
	q := queuedSet(result)
	if q["testorg/extra"] {
		t.Error("testorg/extra is not in watchRepos and must be filtered out")
	}
	if !q["testorg/keep"] {
		t.Error("testorg/keep is in watchRepos and must be indexed")
	}
	if desired, _ := result["desired"].(int); desired != 1 {
		t.Errorf("desired=%d, want 1 (only the allowlisted repo)", desired)
	}
	if filtered, _ := result["filtered"].(int); filtered != 1 {
		t.Errorf("filtered=%d, want 1 (the excluded App repo)", filtered)
	}
}

// TestWatchReposAllowlistFiltersOrgDiscovery: the same allowlist applies to PAT
// (watchOrgs) discovery, so it behaves identically for token and App auth.
func TestWatchReposAllowlistFiltersOrgDiscovery(t *testing.T) {
	t.Setenv("GITHUB_API_URL", githubMock(t, 200, []githubRepo{
		{FullName: "testorg/keep", CloneURL: "https://x/keep.git", DefaultBranch: "main"},
		{FullName: "testorg/extra", CloneURL: "https://x/extra.git", DefaultBranch: "main"},
	}))
	shard := newFakeShard(true)
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{
		backends: backends, timeout: time.Second, concurrency: 1,
		watchRepos: []string{"testorg/keep"}, watchOrgs: []string{"testorg"}, ring: newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()
	q := queuedSet(result)
	if q["testorg/extra"] {
		t.Error("testorg/extra must be filtered out of org discovery by the allowlist")
	}
	if !q["testorg/keep"] {
		t.Error("testorg/keep must be indexed")
	}
	if desired, _ := result["desired"].(int); desired != 1 {
		t.Errorf("desired=%d, want 1", desired)
	}
}

// TestNoWatchReposIndexesAllAppRepos: with watchRepos empty, discovery is
// additive as before (no regression).
func TestNoWatchReposIndexesAllAppRepos(t *testing.T) {
	auth, closeGH := appAuthServing(t, []map[string]any{
		{"full_name": "testorg/keep", "clone_url": "https://x/keep.git", "default_branch": "main"},
		{"full_name": "testorg/extra", "clone_url": "https://x/extra.git", "default_branch": "main"},
	})
	defer closeGH()

	shard := newFakeShard(true)
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{
		backends: backends, timeout: time.Second, concurrency: 1,
		auth: auth, ring: newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()
	if desired, _ := result["desired"].(int); desired != 2 {
		t.Errorf("desired=%d, want 2 (additive when no allowlist)", desired)
	}
}
