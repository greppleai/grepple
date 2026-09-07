package router

import (
	"fmt"
	"grepple/internal/api"
	"reflect"
	"testing"
)

func desiredSet(n int) map[string]api.RepoRef {
	d := map[string]api.RepoRef{}
	for i := 0; i < n; i++ {
		repo := fmt.Sprintf("acme/repo%03d", i)
		d[repo] = api.RepoRef{Repo: repo}
	}
	return d
}

func TestSelectReposToEnqueueBatchesAndDefers(t *testing.T) {
	desired := desiredSet(10)
	queue, deferred := selectReposToEnqueue(desired, nil, nil, 3)
	if len(queue) != 3 || deferred != 7 {
		t.Fatalf("batch=3 over 10 desired: queue=%d deferred=%d", len(queue), deferred)
	}
	// Deterministic, sorted selection so batching advances predictably.
	want := []string{"acme/repo000", "acme/repo001", "acme/repo002"}
	if !reflect.DeepEqual(queue, want) {
		t.Fatalf("queue=%v want %v", queue, want)
	}
}

func TestSelectReposToEnqueueUnlimited(t *testing.T) {
	desired := desiredSet(10)
	queue, deferred := selectReposToEnqueue(desired, nil, nil, 0)
	if len(queue) != 10 || deferred != 0 {
		t.Fatalf("batch=0 (unlimited): queue=%d deferred=%d", len(queue), deferred)
	}
}

func TestSelectReposToEnqueueSkipsPresentAndInflight(t *testing.T) {
	desired := desiredSet(6) // repo000..repo005
	present := map[string]bool{"acme/repo000": true, "acme/repo001": true}
	inflight := map[string]bool{"acme/repo002": true}
	queue, deferred := selectReposToEnqueue(desired, present, inflight, 0)
	want := []string{"acme/repo003", "acme/repo004", "acme/repo005"}
	if !reflect.DeepEqual(queue, want) {
		t.Fatalf("queue=%v want %v (present/inflight must be skipped)", queue, want)
	}
	if deferred != 0 {
		t.Fatalf("deferred=%d want 0", deferred)
	}
}

// TestSelectReposToEnqueueCrossCycleProgress simulates successive reconcile
// cycles: each cycle enqueues a batch, those become in-flight, and the next
// cycle advances to the next batch rather than resubmitting the same repos.
func TestSelectReposToEnqueueCrossCycleProgress(t *testing.T) {
	seen, cycles := simulateBatchedCycles(t, desiredSet(10), 4)

	if len(seen) != 10 {
		t.Fatalf("expected all 10 repos enqueued exactly once, got %d distinct", len(seen))
	}
	for repo, n := range seen {
		if n != 1 {
			t.Fatalf("repo %s enqueued %d times; want exactly once", repo, n)
		}
	}
	if cycles != 3 { // 4 + 4 + 2
		t.Fatalf("expected 3 cycles for 10 repos at batch 4, got %d", cycles)
	}
}

// simulateBatchedCycles runs successive reconcile cycles: each enqueues a
// batch, those become in-flight, and the next cycle's completed work becomes
// present — until nothing is left. It returns enqueue counts per repo and the
// number of cycles, failing if batching never converges or over-enqueues.
func simulateBatchedCycles(t *testing.T, desired map[string]api.RepoRef, batch int) (map[string]int, int) {
	t.Helper()
	present := map[string]bool{}
	inflight := map[string]bool{}
	seen := map[string]int{}
	cycles := 0
	for {
		queue, _ := selectReposToEnqueue(desired, present, inflight, batch)
		if len(queue) == 0 {
			break
		}
		cycles++
		if cycles > 100 {
			t.Fatal("did not converge; batching is re-enqueuing in-flight repos")
		}
		if len(queue) > batch {
			t.Fatalf("cycle enqueued %d > batch %d", len(queue), batch)
		}
		for _, repo := range queue {
			seen[repo]++
			// Model the job entering flight until the next cycle indexes it.
			inflight[repo] = true
		}
		// Previous cycle's in-flight work finishes and becomes present.
		for repo := range inflight {
			if seen[repo] >= 1 {
				present[repo] = true
			}
		}
		inflight = map[string]bool{}
	}
	return seen, cycles
}
