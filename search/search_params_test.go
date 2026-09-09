package search

import (
	"github.com/greppleai/grepple/api"
	"testing"
)

// TestEnforcePageLimit verifies the public paging policy: unset limit resolves
// to the default page size, explicit 0 ("all") and oversized limits clamp to
// the hard cap, and in-range values pass through untouched. ResolveRequest
// alone stays uncapped so internal fan-out windows (skip+limit) survive.
func TestEnforcePageLimit(t *testing.T) {
	ptr := func(n int) *int { return &n }
	query := "deploy"
	cases := []struct {
		name  string
		limit *int
		want  int
	}{
		{"unset defaults", nil, DefaultPageLimit},
		{"zero clamps to cap", ptr(0), MaxPageLimit},
		{"negative clamps to cap", ptr(-3), MaxPageLimit},
		{"one passes through", ptr(1), 1},
		{"in range passes through", ptr(50), 50},
		{"at cap passes through", ptr(MaxPageLimit), MaxPageLimit},
		{"above cap clamps", ptr(MaxPageLimit + 1), MaxPageLimit},
		{"huge clamps", ptr(1_000_000), MaxPageLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := ResolveRequest(api.SearchRequest{Query: &query, Limit: c.limit})
			if err != nil {
				t.Fatalf("ResolveRequest: %v", err)
			}
			EnforcePageLimit(&p, api.SearchRequest{Query: &query, Limit: c.limit})
			if p.Limit != c.want {
				t.Fatalf("limit %v -> %d, want %d", c.limit, p.Limit, c.want)
			}
		})
	}
}

// TestResolveRequestStaysUncapped guards the internal contract: ResolveRequest
// must not clamp on its own, or the router's skip+limit fan-out windows would
// collapse and deep pages would lose results.
func TestResolveRequestStaysUncapped(t *testing.T) {
	query := "deploy"
	p, err := ResolveRequest(api.SearchRequest{Query: &query, Limit: func() *int { n := 5000; return &n }()})
	if err != nil {
		t.Fatalf("ResolveRequest: %v", err)
	}
	if p.Limit != 5000 {
		t.Fatalf("ResolveRequest clamped an internal window: got %d, want 5000", p.Limit)
	}
}

func TestResolveRequestAsymmetricContext(t *testing.T) {
	query := "deploy"
	before, after := 1, 5
	p, err := ResolveRequest(api.SearchRequest{
		Query:         &query,
		Context:       float64(3),
		BeforeContext: &before,
		AfterContext:  &after,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Context != 3 || p.BeforeContext != 1 || p.AfterContext != 5 {
		t.Fatalf("unexpected resolved context: %#v", p)
	}
}
