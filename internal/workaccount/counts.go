// Package workaccount holds per-request, opt-in counts of observed local work.
// It never records source names or values and is safe for parallel workers.
package workaccount

import (
	"context"
	"sync/atomic"
)

type key struct{}

type Counters struct {
	candidates         atomic.Int64
	selected           atomic.Int64
	attempted          atomic.Int64
	read               atomic.Int64
	readFailed         atomic.Int64
	bytesRead          atomic.Int64
	parseAttempted     atomic.Int64
	parsed             atomic.Int64
	parseFailed        atomic.Int64
	candidatesObserved atomic.Bool
	selectedObserved   atomic.Bool
	parseObserved      atomic.Bool
}

// With attaches fresh invocation-owned counters. Never share a Counters between invocations.
func With(ctx context.Context) (context.Context, *Counters) {
	counts := &Counters{}
	return context.WithValue(ctx, key{}, counts), counts
}

// From returns nil when observation was not explicitly enabled.
func From(ctx context.Context) *Counters {
	if ctx == nil {
		return nil
	}
	counts, _ := ctx.Value(key{}).(*Counters)
	return counts
}

// Candidates records a completed initial collection, not scan attempts.
func (c *Counters) Candidates(n int) {
	if c != nil && n >= 0 {
		c.candidates.Store(int64(n))
		c.candidatesObserved.Store(true)
	}
}

// Selected records paths admitted by a listing (before output paging).
func (c *Counters) Selected(n int) {
	if c != nil && n >= 0 {
		c.selected.Store(int64(n))
		c.selectedObserved.Store(true)
	}
}

// ReadAttempt records an actual content-read call, even when it fails.
func (c *Counters) ReadAttempt(bytes int, err error) {
	if c == nil {
		return
	}
	c.attempted.Add(1)
	if err != nil {
		c.readFailed.Add(1)
		return
	}
	c.read.Add(1)
	c.bytesRead.Add(int64(bytes))
}

// ParsingApplicable marks an instrumented parse path, including a path with zero attempts.
func (c *Counters) ParsingApplicable() {
	if c != nil {
		c.parseObserved.Store(true)
	}
}

// ParseAttempt records one actual parser invocation, not a language lookup or cached snapshot.
func (c *Counters) ParseAttempt(err error) {
	if c == nil {
		return
	}
	c.parseObserved.Store(true)
	c.parseAttempted.Add(1)
	if err != nil {
		c.parseFailed.Add(1)
	} else {
		c.parsed.Add(1)
	}
}

// Snapshot contains only aggregate counts and applicability flags, never paths or content.
type Snapshot struct {
	Candidates, Selected, Attempted, Read, ReadFailed, BytesRead int64
	ParseAttempted, Parsed, ParseFailed                          int64
	CandidatesObserved, SelectedObserved, ParseObserved          bool
}

func (c *Counters) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return Snapshot{
		Candidates: c.candidates.Load(), Selected: c.selected.Load(), Attempted: c.attempted.Load(),
		Read: c.read.Load(), ReadFailed: c.readFailed.Load(), BytesRead: c.bytesRead.Load(),
		ParseAttempted: c.parseAttempted.Load(), Parsed: c.parsed.Load(), ParseFailed: c.parseFailed.Load(),
		CandidatesObserved: c.candidatesObserved.Load(), SelectedObserved: c.selectedObserved.Load(), ParseObserved: c.parseObserved.Load(),
	}
}
