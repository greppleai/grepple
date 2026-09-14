package search

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestMeasureNavigationResolutionIsDeterministicByLanguageAndConfidence(t *testing.T) {
	graph := parser.NavigationGraph{Calls: []parser.NavigationCall{
		{Language: "tsx", Confidence: "candidate", CandidateTargetIDs: []string{"one", "two"}},
		{Language: "go", Confidence: "exact", TargetID: "target"},
		{Language: "typescript", Confidence: "candidate"},
		{Language: "go", Confidence: "candidate", CandidateTargetIDs: []string{"one"}},
	}}
	stats := MeasureNavigationResolution(graph)
	if stats.Calls != 4 || stats.Resolved != 1 || stats.Ambiguous != 1 || stats.Unresolved != 1 || stats.Candidate != 1 || stats.AmbiguityRate != 0.25 {
		t.Fatalf("resolution stats=%+v", stats)
	}
	if got, want := stats.Confidences, []NavigationResolutionCount{{Name: "candidate", Count: 3}, {Name: "exact", Count: 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("confidence counts=%+v want %+v", got, want)
	}
	if len(stats.Languages) != 2 || stats.Languages[0].Language != "go" || stats.Languages[1].Language != "typescript" || stats.Languages[1].Calls != 2 {
		t.Fatalf("language stats=%+v", stats.Languages)
	}
}
