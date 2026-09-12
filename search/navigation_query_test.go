package search

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestQueryNavigationGraphTraversesCalleesByDepthAndStopsCycles(t *testing.T) {
	graph := navigationQueryFixture()
	one, err := QueryNavigationGraph(graph, []string{"root"}, NavigationQueryCallees, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(one), []string{"root", "a", "candidate-one", "candidate-two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-one declarations=%v, want %v", got, want)
	}
	if got, want := queryCallIDs(one), []string{"root-a", "root-candidates"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-one calls=%v, want %v", got, want)
	}

	two, err := QueryNavigationGraph(graph, []string{"root"}, NavigationQueryCallees, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(two), []string{"root", "a", "b", "candidate-one", "candidate-two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-two declarations=%v, want %v", got, want)
	}
	if got, want := queryCallIDs(two), []string{"root-a", "a-b", "root-candidates"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-two calls=%v, want %v", got, want)
	}
}

func TestQueryNavigationGraphTraversesCallersAndRetainsCandidateContext(t *testing.T) {
	graph := navigationQueryFixture()
	result, err := QueryNavigationGraph(graph, []string{"root"}, NavigationQueryCallers, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(result), []string{"root", "a", "b", "incoming", "incoming-parent", "candidate-two", "candidate-caller"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declarations=%v, want %v", got, want)
	}
	if got, want := queryCallIDs(result), []string{"a-b", "b-root", "incoming-root", "parent-incoming", "candidate-root"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
}

func TestQueryNavigationGraphCandidateContextDoesNotStopCallerTraversal(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "root"}, {ID: "candidate"}, {ID: "caller"}, {ID: "parent"},
		},
		Calls: []parser.NavigationCall{
			{ID: "ambiguous", CallerID: "caller", CandidateTargetIDs: []string{"root", "candidate"}, Confidence: "candidate"},
			{ID: "candidate-caller", CallerID: "candidate", TargetID: "caller", Confidence: "exact"},
			{ID: "parent-candidate", CallerID: "parent", TargetID: "candidate", Confidence: "exact"},
		},
	}
	result, err := QueryNavigationGraph(graph, []string{"root"}, NavigationQueryCallers, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := queryDeclarationIDs(result), []string{"root", "candidate", "caller", "parent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declarations=%v, want %v", got, want)
	}
	if got, want := queryCallIDs(result), []string{"ambiguous", "candidate-caller", "parent-candidate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
}

func TestQueryNavigationGraphRejectsInvalidQueries(t *testing.T) {
	graph := navigationQueryFixture()
	for _, test := range []struct {
		roots     []string
		direction NavigationQueryDirection
		depth     int
	}{
		{nil, NavigationQueryCallers, 1},
		{[]string{"missing"}, NavigationQueryCallers, 1},
		{[]string{"root"}, "sideways", 1},
		{[]string{"root"}, NavigationQueryCallees, 0},
	} {
		if _, err := QueryNavigationGraph(graph, test.roots, test.direction, test.depth); err == nil {
			t.Fatalf("query %#v unexpectedly succeeded", test)
		}
	}
}

func navigationQueryFixture() parser.NavigationGraph {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Root", Path: "root.go"},
		{ID: "a", Name: "A", Path: "a.go"},
		{ID: "b", Name: "B", Path: "b.go"},
		{ID: "incoming", Name: "Incoming", Path: "incoming.go"},
		{ID: "incoming-parent", Name: "IncomingParent", Path: "parent.go"},
		{ID: "candidate-one", Name: "Candidate", Path: "one.go"},
		{ID: "candidate-two", Name: "Candidate", Path: "two.go"},
		{ID: "candidate-caller", Name: "CandidateCaller", Path: "candidate.go"},
	}
	calls := []parser.NavigationCall{
		{ID: "root-a", CallerID: "root", TargetID: "a", Confidence: "exact"},
		{ID: "a-b", CallerID: "a", TargetID: "b", Confidence: "exact"},
		{ID: "b-root", CallerID: "b", TargetID: "root", Confidence: "exact"},
		{ID: "incoming-root", CallerID: "incoming", TargetID: "root", Confidence: "exact"},
		{ID: "parent-incoming", CallerID: "incoming-parent", TargetID: "incoming", Confidence: "exact"},
		{ID: "root-candidates", CallerID: "root", CandidateTargetIDs: []string{"candidate-one", "candidate-two"}, Confidence: "candidate"},
		{ID: "candidate-root", CallerID: "candidate-caller", CandidateTargetIDs: []string{"root", "candidate-two"}, Confidence: "candidate"},
	}
	return parser.NavigationGraph{Declarations: declarations, Calls: calls}
}

func queryDeclarationIDs(graph parser.NavigationGraph) []string {
	ids := make([]string, 0, len(graph.Declarations))
	for _, declaration := range graph.Declarations {
		ids = append(ids, declaration.ID)
	}
	return ids
}

func queryCallIDs(graph parser.NavigationGraph) []string {
	ids := make([]string, 0, len(graph.Calls))
	for _, call := range graph.Calls {
		ids = append(ids, call.ID)
	}
	return ids
}
