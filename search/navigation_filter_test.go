package search

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestFilterNavigationGraphPreservesOnlyEligibleClosedEdges(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "go-root", Language: "go"},
			{ID: "go-target", Language: "go"},
			{ID: "python-target", Language: "python"},
		},
		Calls: []parser.NavigationCall{
			{ID: "exact", CallerID: "go-root", TargetID: "go-target", Language: "go", Confidence: "exact"},
			{ID: "candidate", CallerID: "go-root", CandidateTargetIDs: []string{"go-target", "python-target"}, Language: "go", Confidence: "candidate"},
			{ID: "cross", CallerID: "go-root", TargetID: "python-target", Language: "go", Confidence: "exact"},
			{ID: "python", CallerID: "python-target", Language: "python", Confidence: "candidate"},
		},
	}
	filtered, err := FilterNavigationGraph(graph, NavigationGraphFilter{Languages: []string{"go"}, Confidences: []string{"candidate"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(filtered), []string{"go-root", "go-target"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declarations=%v, want %v", got, want)
	}
	if got, want := queryCallIDs(filtered), []string{"candidate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	if got, want := filtered.Calls[0].CandidateTargetIDs, []string{"go-target"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate IDs=%v, want %v", got, want)
	}
}

func TestNormalizeNavigationGraphFilterValidatesSortsAndDeduplicates(t *testing.T) {
	filter, err := NormalizeNavigationGraphFilter(NavigationGraphFilter{
		Languages:    []string{"python", "go", "python"},
		Confidences:  []string{"unique-terminal", "exact", "exact"},
		Visibilities: []string{"unknown", "public", "public"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filter.Languages, []string{"go", "python"}) || !reflect.DeepEqual(filter.Confidences, []string{"exact", "unique-terminal"}) || !reflect.DeepEqual(filter.Visibilities, []string{"public", "unknown"}) {
		t.Fatalf("filter=%#v", filter)
	}
	for _, invalid := range []NavigationGraphFilter{{Languages: []string{"text"}}, {Confidences: []string{"likely"}}, {Visibilities: []string{"maybe"}}} {
		if _, err := NormalizeNavigationGraphFilter(invalid); err == nil {
			t.Fatalf("filter %#v unexpectedly succeeded", invalid)
		}
	}
}

func TestFilterNavigationGraphByVisibilityPreservesClosedGraph(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "public", Language: "go", Visibility: parser.NavigationVisibilityPublic},
			{ID: "private", Language: "go", Visibility: parser.NavigationVisibilityNonPublic},
		},
		Calls: []parser.NavigationCall{{ID: "edge", CallerID: "public", TargetID: "private", Language: "go", Confidence: "exact"}},
	}
	filtered, err := FilterNavigationGraph(graph, NavigationGraphFilter{Visibilities: []string{"public"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(filtered), []string{"public"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declarations=%v, want %v", got, want)
	}
	if len(filtered.Calls) != 0 {
		t.Fatalf("calls=%#v, want closed graph without private target", filtered.Calls)
	}
}
