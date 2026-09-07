package router

import (
	"fmt"
	"grepple/internal/api"
	"sort"
	"sync"
	"time"
)

type jobRecord struct {
	ID         int    `json:"id"`
	Event      string `json:"event"`
	Repo       string `json:"repo"`
	Method     string `json:"method"`
	Shard      string `json:"shard"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Error      string `json:"error,omitempty"`
	EnqueuedAt string `json:"enqueuedAt"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

type indexJob struct {
	id            int
	event, method string
	ref           api.RepoRef
	attempt       int
}

type jobQueue struct {
	o            routerOptions
	mu           sync.Mutex
	pending      []indexJob
	records      map[int]*jobRecord
	active, next int
}

func newQueue(o routerOptions) *jobQueue {
	return &jobQueue{o: o, records: map[int]*jobRecord{}, next: 1}
}

func (q *jobQueue) enqueue(event, method string, ref api.RepoRef) int {
	q.mu.Lock()
	id := q.next
	q.next++
	q.records[id] = &jobRecord{ID: id, Event: event, Repo: ref.Repo, Method: method, Shard: q.o.ring.get(ref.Repo), Status: "queued", EnqueuedAt: isoNow()}
	q.pending = append(q.pending, indexJob{id, event, method, ref, 1})
	q.mu.Unlock()
	q.pump()
	return id
}

// pendingRepos returns the set of repositories that currently have an
// unfinished job (queued or running). The reconciler uses it to avoid
// re-enqueuing work that is already in flight, so batched initial syncs make
// forward progress across cycles instead of resubmitting the same repos.
func (q *jobQueue) pendingRepos() map[string]bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	repos := map[string]bool{}
	for _, r := range q.records {
		if r.Status == "queued" || r.Status == "running" {
			repos[r.Repo] = true
		}
	}
	return repos
}

func (q *jobQueue) pump() {
	q.mu.Lock()
	for q.active < q.o.concurrency && len(q.pending) > 0 {
		j := q.pending[0]
		q.pending = q.pending[1:]
		q.active++
		go q.run(j)
	}
	q.mu.Unlock()
}

func (q *jobQueue) run(j indexJob) {
	q.mu.Lock()
	q.records[j.id].Status = "running"
	q.mu.Unlock()
	status, m := indexShard(q.o, q.o.ring.get(j.ref.Repo), j.method, j.ref)
	if status >= 500 && j.attempt < 6 {
		delay := 500 * time.Millisecond * time.Duration(1<<(j.attempt-1))
		q.mu.Lock()
		r := q.records[j.id]
		r.Status = "queued"
		r.HTTPStatus = status
		r.Error = fmt.Sprintf("transient failure (%d); retry %d/6 in %dms", status, j.attempt+1, delay.Milliseconds())
		q.active--
		q.mu.Unlock()
		time.AfterFunc(delay, func() {
			q.mu.Lock()
			j.attempt++
			q.pending = append(q.pending, j)
			q.mu.Unlock()
			q.pump()
		})
		q.pump()
		return
	}
	q.mu.Lock()
	r := q.records[j.id]
	if status < 400 {
		r.Status = "done"
	} else {
		r.Status = "error"
		r.Error = fmt.Sprint(m["error"])
	}
	r.HTTPStatus = status
	r.FinishedAt = isoNow()
	q.active--
	q.mu.Unlock()
	q.pump()
}

func (q *jobQueue) stats() map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	var rec []jobRecord
	for _, r := range q.records {
		rec = append(rec, *r)
	}
	sort.Slice(rec, func(i, j int) bool {
		return rec[i].ID > rec[j].ID
	})
	if len(rec) > 50 {
		rec = rec[:50]
	}
	return map[string]any{"pending": len(q.pending), "active": q.active, "recent": rec}
}
