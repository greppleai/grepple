package navigation

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestMeasureNavigationResolutionIsDeterministicByLanguageAndConfidence(t *testing.T) {
	graph := parser.NavigationGraph{Calls: []parser.NavigationCall{
		{Language: "tsx", Confidence: "candidate", CandidateTargetIDs: []string{"one", "two"}},
		{Language: "go", Confidence: "exact", TargetID: "target"},
		{Language: "typescript", Confidence: "candidate"},
		{Language: "go", Confidence: "candidate", CandidateTargetIDs: []string{"one"}},
		{Language: "typescript", Confidence: "candidate", ImportPath: "external-package"},
	}}
	stats := measureNavigationResolution(graph)
	if stats.Calls != 5 || stats.Resolved != 1 || stats.Ambiguous != 1 || stats.Unresolved != 2 || stats.Candidate != 1 || stats.AmbiguityRate != 0.2 {
		t.Fatalf("resolution stats=%+v", stats)
	}
	if got, want := stats.Confidences, []NavigationResolutionCount{{Name: "candidate", Count: 4}, {Name: "exact", Count: 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("confidence counts=%+v want %+v", got, want)
	}
	if got, want := stats.Outcomes, []NavigationResolutionCount{{Name: "ambiguous-local", Count: 1}, {Name: "expected-external", Count: 1}, {Name: "resolved-local", Count: 1}, {Name: "unresolved-local", Count: 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcome counts=%+v want %+v", got, want)
	}
	if stats.ResolvedLocal != 1 || stats.AmbiguousLocal != 1 || stats.UnresolvedLocal != 2 || stats.ExpectedExternal != 1 || stats.ResolutionRate != 0.2 || stats.AmbiguousLocalRate != 0.2 || stats.UnresolvedLocalRate != 0.4 || stats.ExpectedExternalRate != 0.2 {
		t.Fatalf("outcome stats=%+v", stats)
	}
	if len(stats.Languages) != 2 || stats.Languages[0].Language != "go" || stats.Languages[1].Language != "typescript" || stats.Languages[1].Calls != 3 || stats.Languages[1].ExpectedExternal != 1 {
		t.Fatalf("language stats=%+v", stats.Languages)
	}
}
