package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchCountSortIsDeterministicAndOptIn(t *testing.T) {
	directory := t.TempDir()
	low := filepath.Join(directory, "a-low.txt")
	high := filepath.Join(directory, "z-high.txt")
	if err := os.WriteFile(low, []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(high, []byte("needle\nneedle\nneedle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates := []string{high, low}
	pathResults, err := Files(Params{Query: "needle", Limit: 1, MaxSegments: DefaultMaxSegments, Sort: ResultSortPath}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(pathResults) != 1 || !strings.HasSuffix(pathResults[0].DisplayPath, "a-low.txt") {
		t.Fatalf("default path order=%#v", pathResults)
	}
	matchResults, err := Files(Params{Query: "needle", Limit: 1, MaxSegments: DefaultMaxSegments, Sort: ResultSortMatches}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(matchResults) != 1 || !strings.HasSuffix(matchResults[0].DisplayPath, "z-high.txt") || len(matchResults[0].MatchLines) != 3 {
		t.Fatalf("match-count order=%#v", matchResults)
	}
}

func TestMatchCountSortUsesPathAsTieBreaker(t *testing.T) {
	matches := []FileMatch{
		{DisplayPath: "b.go", MatchLines: map[int]bool{1: true, 2: true}},
		{DisplayPath: "a.go", MatchLines: map[int]bool{3: true, 4: true}},
	}
	sortMatches(matches, ResultSortMatches)
	if matches[0].DisplayPath != "a.go" || matches[1].DisplayPath != "b.go" {
		t.Fatalf("tie order=%#v", matches)
	}
}
