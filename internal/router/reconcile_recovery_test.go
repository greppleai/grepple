package router

import (
	"context"
	"encoding/json"
	"grepple/internal/api"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeShard is a minimal stand-in for a search shard. It tracks the
// repositories it has indexed and can be toggled unhealthy, in which case index
// writes fail permanently (HTTP 400, so the router does not retry) — modelling a
// shard whose startup index attempts did not succeed.
type fakeShard struct {
	mu           sync.Mutex
	repos        map[string]api.RepoInfo
	healthy      atomic.Bool
	failedWrites atomic.Int32
	puts         atomic.Int32
}

func newFakeShard(healthy bool) *fakeShard {
	d := &fakeShard{repos: map[string]api.RepoInfo{}}
	d.healthy.Store(healthy)
	return d
}

func (d *fakeShard) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.repos)
}

func (d *fakeShard) has(repo string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.repos[repo]
	return ok
}

func (d *fakeShard) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/index" && r.Method == http.MethodGet:
			d.serveIndexList(w)
		case r.URL.Path == "/index" && (r.Method == http.MethodPost || r.Method == http.MethodPut):
			d.serveIndexWrite(w, r)
		case r.URL.Path == "/index" && r.Method == http.MethodDelete:
			d.serveIndexDelete(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// serveIndexList returns the shard's indexed repositories.
func (d *fakeShard) serveIndexList(w http.ResponseWriter) {
	d.mu.Lock()
	list := make([]api.RepoInfo, 0, len(d.repos))
	for _, info := range d.repos {
		list = append(list, info)
	}
	d.mu.Unlock()
	_ = json.NewEncoder(w).Encode(api.IndexListResponse{Repos: list})
}

// serveIndexWrite records an index/refresh of one repo; an unhealthy shard
// answers 400 (the router treats it as a failed index with no retry).
func (d *fakeShard) serveIndexWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		d.puts.Add(1)
	}
	if !d.healthy.Load() {
		d.failedWrites.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"shard unavailable"}`))
		return
	}
	body, _ := io.ReadAll(r.Body)
	var ref api.RepoRef
	_ = json.Unmarshal(body, &ref)
	d.mu.Lock()
	d.repos[ref.Repo] = api.RepoInfo{Repo: ref.Repo, URL: ref.URL, Ref: ref.Ref, IndexedAt: isoNow()}
	d.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// serveIndexDelete drops one repo; like the real shard, the repo comes from
// the JSON body, falling back to the ?repository= query param.
func (d *fakeShard) serveIndexDelete(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repository")
	if body, _ := io.ReadAll(r.Body); len(body) > 0 {
		var ref api.RepoRef
		if json.Unmarshal(body, &ref) == nil && ref.Repo != "" {
			repo = ref.Repo
		}
	}
	d.mu.Lock()
	delete(d.repos, repo)
	d.mu.Unlock()
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// TestReconcileLoopHealsUnreachableShard reproduces the startup imbalance bug:
// a shard that is unreachable when the router first reconciles never receives
// its repositories, because the one-shot startup reconcile is not retried. With
// the periodic reconcile loop, the repo is re-enqueued once the shard recovers,
// so the shards become balanced again without manual intervention.
func TestReconcileLoopHealsUnreachableShard(t *testing.T) {
	shardA := newFakeShard(true)
	shardB := newFakeShard(true)
	serverA := httptest.NewServer(shardA.handler())
	serverB := httptest.NewServer(shardB.handler())
	defer serverA.Close()
	defer serverB.Close()

	backends := []string{serverA.URL, serverB.URL}
	ring := newRing(backends)

	const repo = "org/needs-shard"
	target := ring.get(repo)
	shards := map[string]*fakeShard{serverA.URL: shardA, serverB.URL: shardB}
	down := shards[target]

	// Simulate the target shard being unreachable during startup.
	down.healthy.Store(false)

	options := routerOptions{
		backends:          backends,
		timeout:           time.Second,
		concurrency:       1,
		watchRepos:        []string{repo},
		reconcileInterval: 20 * time.Millisecond,
		ring:              ring,
	}
	state := &routerState{o: options}
	state.q = newQueue(options)

	// Start the reconcile loop while the shard is unreachable. Its initial
	// (startup) reconcile attempts to index the repo and fails; without the
	// periodic loop this would be the only attempt and the repo would stay
	// missing forever, leaving the shards permanently unbalanced.
	stopCtx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() {
		state.reconcileLoop(stopCtx)
		close(loopDone)
	}()

	// Wait until the startup reconcile has actually attempted and failed to
	// index on the down shard before allowing it to recover. This guarantees
	// that healing can only come from a subsequent periodic reconcile.
	if !waitFor(2*time.Second, func() bool { return down.failedWrites.Load() >= 1 }) {
		t.Fatalf("startup reconcile never attempted to index on the down shard")
	}
	if down.has(repo) {
		t.Fatalf("repo unexpectedly indexed while shard was down")
	}

	// Shard recovers. Only a later periodic reconcile can now heal the imbalance.
	down.healthy.Store(true)

	if !waitFor(2*time.Second, func() bool { return down.has(repo) }) {
		t.Fatalf("recovered shard %s never received repo after periodic reconcile", target)
	}

	cancel()
	select {
	case <-loopDone:
	case <-time.After(time.Second):
		t.Fatalf("reconcile loop did not stop after context cancellation")
	}

	// The repo lives on exactly one shard: its ring target. No duplicate/imbalance.
	if shardA.count()+shardB.count() != 1 {
		t.Fatalf("expected repo on exactly one shard, got A=%d B=%d", shardA.count(), shardB.count())
	}
}

func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}
