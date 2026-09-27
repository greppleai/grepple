package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestCompactCallersGroupsCallSitesUnderTheRoot(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "target", Name: "parseAtRange", Kind: "func", Language: "go", Path: "search/at.go", Start: 150, End: 166},
		{ID: "a", Name: "At", Kind: "func", Language: "go", Path: "search/at.go", Start: 15, End: 70},
		{ID: "b", Name: "AtFromDocument", Kind: "func", Language: "go", Path: "search/at.go", Start: 72, End: 114},
		{ID: "c", Name: "parseAtReference", Kind: "func", Language: "go", Path: "search/at.go", Start: 145, End: 148},
	}
	calls := []parser.NavigationCall{
		{ID: "call-a", CallerID: "a", TargetID: "target", Confidence: "unique-terminal", Path: "search/at.go", Line: 18},
		{ID: "call-b", CallerID: "b", TargetID: "target", Confidence: "unique-terminal", Path: "search/at.go", Line: 75},
		{ID: "call-c", CallerID: "c", TargetID: "target", Confidence: "unique-terminal", Path: "search/at.go", Line: 146},
	}
	var output strings.Builder
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "callers", Depth: 1, RootIDs: []string{"target"}}, declarations, calls)
	want := "go func parseAtRange search/at.go:150-166\n" +
		"<- func At search/at.go:15-70 call:18 [unique-terminal]\n" +
		"<- func AtFromDocument search/at.go:72-114 call:75 [unique-terminal]\n" +
		"<- func parseAtReference search/at.go:145-148 call:146 [unique-terminal]\n"
	if got := output.String(); got != want {
		t.Fatalf("compact callers:\n%s\nwant:\n%s", got, want)
	}
}

func TestCompactDirectionalQueryDisplaysAvailableSignaturesByDefault(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Start", Signature: "func Start(value Input) (Output, error)", Kind: "func", Language: "go", Path: "start.go", Start: 2, End: 4},
		{ID: "target", Name: "Load", Signature: "func Load(input Input) Output", Kind: "func", Language: "go", Path: "load.go", Start: 5, End: 8},
	}
	calls := []parser.NavigationCall{{ID: "call", CallerID: "root", TargetID: "target", Confidence: "exact", Path: "start.go", Line: 3}}
	for _, test := range []struct{ direction, rootID, want string }{
		{"callees", "root", "go func Start(value Input) (Output, error) @ start.go:2-4\n-> func Load(input Input) Output @ load.go:5-8 call:3 [exact]\n"},
		{"callers", "target", "go func Load(input Input) Output @ load.go:5-8\n<- func Start(value Input) (Output, error) @ start.go:2-4 call:3 [exact]\n"},
	} {
		var output strings.Builder
		writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: test.direction, Depth: 1, RootIDs: []string{test.rootID}}, declarations, calls)
		if got := output.String(); got != test.want {
			t.Fatalf("%s signature output:\n%s\nwant:\n%s", test.direction, got, test.want)
		}
	}
}

func TestCompactCallableLabelKeepsNamesForAnonymousSyntax(t *testing.T) {
	declaration := parser.NavigationDeclaration{Name: "run", Signature: "(value: number): number =>", Kind: "func", Path: "run.ts", Start: 1, End: 1}
	if got, want := graphCallableLabel(declaration), "run (value: number): number => @ run.ts:1"; got != want {
		t.Fatalf("label=%q want=%q", got, want)
	}
}

func TestCompactCalleesPreservesAmbiguityAndBoundsCycles(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "root", Kind: "func", Language: "go", Path: "root.go", Start: 2, End: 2},
		{ID: "one", Name: "one", Kind: "func", Language: "go", Path: "one.go", Start: 2, End: 2},
		{ID: "two", Name: "two", Kind: "func", Language: "go", Path: "two.go", Start: 2, End: 2},
	}
	calls := []parser.NavigationCall{
		{ID: "ambiguous", CallerID: "root", CandidateTargetIDs: []string{"one", "two"}, Confidence: "candidate", Path: "root.go", Line: 2},
		{ID: "cycle", CallerID: "one", TargetID: "root", Confidence: "exact", Path: "one.go", Line: 2},
	}
	var output strings.Builder
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "callees", Depth: 3, RootIDs: []string{"root"}}, declarations, calls)
	got := output.String()
	for _, want := range []string{
		"go func root root.go:2\n",
		"-> ? func one one.go:2 call:2 [candidate]\n",
		"  -> func root root.go:2 call:2 [exact] (already shown)\n",
		"-> ? func two two.go:2 call:2 [candidate]\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in compact callees:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") != 4 {
		t.Fatalf("cycle expanded or candidate edge omitted:\n%s", got)
	}
}

func TestCompactImpactGroupsBidirectionalHopsAndCandidates(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Middle", Kind: "func", Language: "go", Path: "flow.go", Start: 2, End: 2},
		{ID: "caller", Name: "Root", Kind: "func", Language: "go", Path: "flow.go", Start: 3, End: 3},
		{ID: "upstream", Name: "Caller", Kind: "func", Language: "go", Path: "flow.go", Start: 4, End: 4},
		{ID: "sibling", Name: "Sibling", Kind: "func", Language: "go", Path: "flow.go", Start: 5, End: 5},
		{ID: "callee", Name: "Leaf", Kind: "func", Language: "go", Path: "flow.go", Start: 6, End: 6},
		{ID: "downstream", Name: "Tail", Kind: "func", Language: "go", Path: "flow.go", Start: 7, End: 7},
		{ID: "one", Name: "One", Kind: "func", Language: "go", Path: "flow.go", Start: 8, End: 8},
		{ID: "two", Name: "Two", Kind: "func", Language: "go", Path: "flow.go", Start: 9, End: 9},
	}
	calls := []parser.NavigationCall{
		{ID: "parent", CallerID: "caller", TargetID: "root", Confidence: "exact", Path: "flow.go", Line: 3},
		{ID: "grandparent", CallerID: "upstream", TargetID: "caller", Confidence: "exact", Path: "flow.go", Line: 4},
		{ID: "branch", CallerID: "caller", TargetID: "sibling", Confidence: "exact", Path: "flow.go", Line: 5},
		{ID: "child", CallerID: "root", TargetID: "callee", Confidence: "exact", Path: "flow.go", Line: 2},
		{ID: "grandchild", CallerID: "callee", TargetID: "downstream", Confidence: "exact", Path: "flow.go", Line: 6},
		{ID: "ambiguous", CallerID: "root", CandidateTargetIDs: []string{"one", "two"}, Confidence: "candidate", Path: "flow.go", Line: 2},
	}
	var output strings.Builder
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "impact", Depth: 2, RootIDs: []string{"root"}}, declarations, calls)
	want := "go func Middle flow.go:2\n" +
		"<- func Root flow.go:3 call:3 [exact]\n" +
		"-> func Leaf flow.go:6 call:2 [exact]\n" +
		"-> ? func One flow.go:8 call:2 [candidate]\n" +
		"-> ? func Two flow.go:9 call:2 [candidate]\n" +
		"  <- func Caller flow.go:4 call:4 [exact]\n" +
		"  -> func Sibling flow.go:5 call:5 [exact]\n" +
		"  -> func Tail flow.go:7 call:6 [exact]\n"
	if got := output.String(); got != want {
		t.Fatalf("grouped impact:\n%s\nwant:\n%s", got, want)
	}
	output.Reset()
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "impact", Depth: 1, RootIDs: []string{"root"}}, declarations, calls)
	if strings.Contains(output.String(), "Caller flow.go:4") || strings.Contains(output.String(), "Sibling flow.go:5") || strings.Contains(output.String(), "Tail flow.go:7") {
		t.Fatalf("impact exceeded depth 1:\n%s", output.String())
	}
}

func TestCompactImpactDoesNotRepeatCallWhenTraversedBothWays(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Root", Kind: "func", Language: "go", Path: "flow.go", Start: 1, End: 1},
		{ID: "other", Name: "Other", Kind: "func", Language: "go", Path: "flow.go", Start: 2, End: 2},
	}
	calls := []parser.NavigationCall{
		{ID: "out", CallerID: "root", TargetID: "other", Confidence: "exact", Path: "flow.go", Line: 1},
		{ID: "back", CallerID: "other", TargetID: "root", Confidence: "candidate", Path: "flow.go", Line: 2},
	}
	var output strings.Builder
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "impact", Depth: 3, RootIDs: []string{"root"}}, declarations, calls)
	got := output.String()
	for _, want := range []string{"<- func Other flow.go:2 call:2 [candidate]", "-> func Other flow.go:2 call:1 [exact] (already shown)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("impact missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "call:1") != 1 || strings.Count(got, "call:2") != 1 {
		t.Fatalf("impact repeated edges while traversing a cycle:\n%s", got)
	}
}
