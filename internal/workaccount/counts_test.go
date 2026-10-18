package workaccount

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRequestCountsConcurrentAndIsolated(t *testing.T) {
	ctx, first := With(context.Background())
	if From(context.Background()) != nil {
		t.Fatal("ambient observer")
	}
	const n = 64
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			From(ctx).ReadAttempt(3, nil)
			From(ctx).ParseAttempt(nil)
		}()
	}
	wg.Wait()
	first.ReadAttempt(0, errors.New("private failure"))
	_, second := With(context.Background())
	if got := first.Snapshot(); got.Attempted != n+1 || got.Read != n || got.ReadFailed != 1 || got.BytesRead != 3*n || got.Parsed != n || got.ParseAttempted != n || got.ParseFailed != 0 {
		t.Fatalf("wrong counts: %+v", got)
	}
	if got := second.Snapshot(); got.Attempted != 0 || got.CandidatesObserved || got.ParseObserved {
		t.Fatalf("counts shared between requests: %+v", got)
	}
}
