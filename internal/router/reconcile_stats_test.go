package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestReconcileCountsArchivedAndDisabled verifies the discovery counters that
// explain the gap between an org's total repositories and what gets indexed:
// archived and disabled repositories are counted and excluded from the desired
// set (so the /reconcile response and logs show why repos are missing).
func TestReconcileCountsArchivedAndDisabled(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode([]githubRepo{
			{FullName: "testorg/active-one", CloneURL: "https://x/active-one.git", DefaultBranch: "main"},
			{FullName: "testorg/active-two", CloneURL: "https://x/active-two.git", DefaultBranch: "main"},
			{FullName: "testorg/old-archived", Archived: true},
			{FullName: "testorg/dead-disabled", Disabled: true},
		})
	}))
	defer github.Close()
	t.Setenv("GITHUB_API_URL", github.URL)

	shard := newFakeShard(true)
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{
		backends:    backends,
		timeout:     time.Second,
		concurrency: 1,
		watchOrgs:   []string{"testorg"},
		ring:        newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}

	result := state.reconcile()
	for key, want := range map[string]int{
		"discovered": 4,
		"archived":   1,
		"disabled":   1,
		"desired":    2, // only the two active repos
	} {
		if got, _ := result[key].(int); got != want {
			t.Fatalf("reconcile %q = %v, want %d", key, result[key], want)
		}
	}
}
