package search

import (
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
			p, err := ResolveRequest(Request{Query: &query, Limit: c.limit})
			if err != nil {
				t.Fatalf("ResolveRequest: %v", err)
			}
			EnforcePageLimit(&p, Request{Query: &query, Limit: c.limit})
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
	p, err := ResolveRequest(Request{Query: &query, Limit: func() *int { n := 5000; return &n }()})
	if err != nil {
		t.Fatalf("ResolveRequest: %v", err)
	}
	if p.Limit != 5000 {
		t.Fatalf("ResolveRequest clamped an internal window: got %d, want 5000", p.Limit)
	}
}

func TestResolveRequestDefaultsToRelatedNavigation(t *testing.T) {
	query := "needle"
	params, err := ResolveRequest(Request{Query: &query})
	if err != nil {
		t.Fatal(err)
	}
	if !params.Related || params.FollowRelated != 1 {
		t.Fatalf("automatic API navigation=%#v", params)
	}
	disabled, err := ResolveRequest(Request{Query: &query, NoRelated: true})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Related || disabled.FollowRelated != 0 || !disabled.NoRelated {
		t.Fatalf("noRelated API navigation=%#v", disabled)
	}
	compact, err := ResolveRequest(Request{Query: &query, SkipSegments: true})
	if err != nil {
		t.Fatal(err)
	}
	if compact.Related || compact.FollowRelated != 0 {
		t.Fatalf("compact API navigation=%#v", compact)
	}
}

func TestResolveRequestAsymmetricContext(t *testing.T) {
	query := "deploy"
	before, after := 1, 5
	p, err := ResolveRequest(Request{
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

func TestResolveRequestEnclosingImpliesLineRanges(t *testing.T) {
	query := "work()"
	params, err := ResolveRequest(Request{Query: &query, EnclosingRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if !params.EnclosingRanges || !params.LineRanges {
		t.Fatalf("unexpected enclosing params: %#v", params)
	}
}

func TestResolveRequestValidatesDeterministicSort(t *testing.T) {
	query := "work"
	defaults, err := ResolveRequest(Request{Query: &query})
	if err != nil || defaults.Sort != ResultSortPath {
		t.Fatalf("default sort=%q err=%v", defaults.Sort, err)
	}
	ranked, err := ResolveRequest(Request{Query: &query, Sort: ResultSortMatches})
	if err != nil || ranked.Sort != ResultSortMatches {
		t.Fatalf("match sort=%q err=%v", ranked.Sort, err)
	}
	if _, err := ResolveRequest(Request{Query: &query, Sort: "score"}); err == nil {
		t.Fatal("expected invalid sort to fail")
	}
}

func TestResolveRequestAcceptsAtWithoutQuery(t *testing.T) {
	params, err := ResolveRequest(Request{At: "app.go:20"})
	if err != nil || params.At != "app.go:20" {
		t.Fatalf("at params=%#v err=%v", params, err)
	}
	query := "work"
	if _, err := ResolveRequest(Request{At: "app.go:20", Query: &query}); err == nil {
		t.Fatal("at with query succeeded")
	}
}

func TestResolveRequestPreservesNavigation(t *testing.T) {
	query := "work"
	params, err := ResolveRequest(Request{Query: &query, FollowRelated: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !params.Related || params.FollowRelated != 2 {
		t.Fatalf("navigation was not preserved: %#v", params)
	}
	if _, err := ResolveRequest(Request{Query: &query, FollowRelated: 4}); err == nil {
		t.Fatal("expected invalid followRelated to fail")
	}
}
