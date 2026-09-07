package search

import "testing"

func TestAggregateRepoCountsTalliesAndSorts(t *testing.T) {
	matches := []FileMatch{
		{DisplayPath: "owner/alpha/x.go", MatchLines: map[int]bool{1: true, 2: true}},
		{DisplayPath: "owner/alpha/y.go", MatchLines: map[int]bool{3: true}},
		{DisplayPath: "owner/beta/z.go", MatchLines: map[int]bool{1: true, 2: true, 3: true, 4: true}},
	}
	got := AggregateRepoCounts(matches)
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d: %#v", len(got), got)
	}
	// beta has more matches (4) so it sorts first; alpha (3 matches / 2 files) second.
	if got[0].Repo != "owner/beta" || got[0].Files != 1 || got[0].Matches != 4 {
		t.Fatalf("unexpected first row: %#v", got[0])
	}
	if got[1].Repo != "owner/alpha" || got[1].Files != 2 || got[1].Matches != 3 {
		t.Fatalf("unexpected second row: %#v", got[1])
	}
}
