package router

import (
	"encoding/json"
	"fmt"
	"grepple/internal/api"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSelectStaleReposToRefresh covers the pure freshness selection: a repo is
// stale when its upstream pushed_at is newer than the shard's indexedAt, and
// the queue stays deterministic and batched.
func TestSelectStaleReposToRefresh(t *testing.T) {
	// Truncate to milliseconds: indexedAt round-trips through isoNow's
	// millisecond-precision format, so raw nanosecond times would compare
	// sub-millisecond "newer" and flag equal timestamps as stale.
	now := time.Now().UTC().Truncate(time.Millisecond)
	iso := func(t time.Time) string { return t.Format("2006-01-02T15:04:05.000Z") }
	indexed := func(repo string, at time.Time) indexedRepo {
		return indexedRepo{RepoInfo: api.RepoInfo{Repo: repo, IndexedAt: iso(at)}, Shard: "shard-a"}
	}

	t.Run("pushed after indexedAt is stale", func(t *testing.T) {
		queue, deferred := selectStaleReposToRefresh(
			map[string]time.Time{"org/a": now},
			[]indexedRepo{indexed("org/a", now.Add(-time.Hour))},
			nil, 0)
		if len(queue) != 1 || queue[0] != "org/a" || deferred != 0 {
			t.Fatalf("queue=%v deferred=%d, want [org/a], 0", queue, deferred)
		}
	})

	t.Run("pushed before or at indexedAt is fresh", func(t *testing.T) {
		queue, _ := selectStaleReposToRefresh(
			map[string]time.Time{"org/a": now, "org/b": now},
			[]indexedRepo{indexed("org/a", now.Add(time.Minute)), indexed("org/b", now)},
			nil, 0)
		if len(queue) != 0 {
			t.Fatalf("queue=%v, want empty", queue)
		}
	})

	t.Run("missing pushed_at data is skipped (allowlist-only repos)", func(t *testing.T) {
		queue, _ := selectStaleReposToRefresh(
			map[string]time.Time{},
			[]indexedRepo{indexed("org/a", now.Add(-time.Hour))},
			nil, 0)
		if len(queue) != 0 {
			t.Fatalf("queue=%v, want empty", queue)
		}
	})

	t.Run("in-flight jobs are not duplicated", func(t *testing.T) {
		queue, _ := selectStaleReposToRefresh(
			map[string]time.Time{"org/a": now},
			[]indexedRepo{indexed("org/a", now.Add(-time.Hour))},
			map[string]bool{"org/a": true}, 0)
		if len(queue) != 0 {
			t.Fatalf("queue=%v, want empty", queue)
		}
	})

	t.Run("missing indexedAt counts as stale", func(t *testing.T) {
		queue, _ := selectStaleReposToRefresh(
			map[string]time.Time{"org/a": now},
			[]indexedRepo{{RepoInfo: api.RepoInfo{Repo: "org/a"}, Shard: "shard-a"}},
			nil, 0)
		if len(queue) != 1 {
			t.Fatalf("queue=%v, want [org/a]", queue)
		}
	})

	t.Run("batch caps the queue and counts deferred, order is deterministic", func(t *testing.T) {
		queue, deferred := selectStaleReposToRefresh(
			map[string]time.Time{"org/c": now, "org/a": now, "org/b": now},
			[]indexedRepo{indexed("org/c", now.Add(-time.Hour)), indexed("org/a", now.Add(-time.Hour)), indexed("org/b", now.Add(-time.Hour))},
			nil, 2)
		if fmt.Sprint(queue) != "[org/a org/b]" || deferred != 1 {
			t.Fatalf("queue=%v deferred=%d, want [org/a org/b], 1", queue, deferred)
		}
	})
}

// TestReconcileRefreshesStaleRepo drives a full reconcile cycle against a fake
// GitHub API and a fake shard: a repo whose pushed_at is newer than the shard's
// indexedAt must get a PUT (pull + reindex) even though it is already present,
// while a repo indexed after its last push must be left alone.
func TestReconcileRefreshesStaleRepo(t *testing.T) {
	now := time.Now().UTC()
	iso := func(t time.Time) string { return t.Format("2006-01-02T15:04:05.000Z") }

	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/orgs/org/repos" {
			_ = json.NewEncoder(w).Encode([]githubRepo{
				{FullName: "org/stale", CloneURL: "https://example.invalid/org/stale.git", DefaultBranch: "main", PushedAt: iso(now)},
				{FullName: "org/fresh", CloneURL: "https://example.invalid/org/fresh.git", DefaultBranch: "main", PushedAt: iso(now.Add(-2 * time.Hour))},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer github.Close()
	t.Setenv("GITHUB_API_URL", github.URL)

	shard := newFakeShard(true)
	shard.repos["org/stale"] = api.RepoInfo{Repo: "org/stale", URL: "https://example.invalid/org/stale.git", Ref: "main", IndexedAt: iso(now.Add(-time.Hour))}
	shard.repos["org/fresh"] = api.RepoInfo{Repo: "org/fresh", URL: "https://example.invalid/org/fresh.git", Ref: "main", IndexedAt: iso(now.Add(-time.Hour))}
	server := httptest.NewServer(shard.handler())
	defer server.Close()

	options := routerOptions{
		backends:       []string{server.URL},
		timeout:        time.Second,
		concurrency:    1,
		watchOrgs:      []string{"org"},
		reconcileBatch: 10,
		ring:           newRing([]string{server.URL}),
	}
	state := &routerState{o: options}
	state.q = newQueue(options)

	state.reconcile()

	if !waitFor(2*time.Second, func() bool { return shard.puts.Load() >= 1 }) {
		t.Fatalf("stale repo never got a refresh PUT")
	}
	// The PUT is a pull+reindex: afterwards the fake shard reports a fresh
	// indexedAt, so a second cycle must not refresh again.
	state.reconcile()
	if got := shard.puts.Load(); got != 1 {
		t.Fatalf("expected exactly one refresh PUT, got %d", got)
	}
	if shard.has("org/stale") != true || shard.has("org/fresh") != true {
		t.Fatalf("both repos must stay indexed, got %+v", shard.repos)
	}
}
