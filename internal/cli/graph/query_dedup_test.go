package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestCompactDirectionalQueryDeduplicatesEvidence(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "caller", Name: "Caller", Kind: "func", Language: "go", Path: "caller.go", Start: 1, End: 10},
		{ID: "target", Name: "Target", Kind: "func", Language: "go", Path: "target.go", Start: 1, End: 10},
		{ID: "other", Name: "Other", Kind: "func", Language: "go", Path: "other.go", Start: 1, End: 10},
	}
	call := parser.NavigationCall{ID: "first", CallerID: "caller", TargetID: "target", Confidence: "exact", Path: "caller.go", Line: 3}
	for _, direction := range []string{"callers", "callees", "impact"} {
		rootID := "caller"
		if direction == "callers" {
			rootID = "target"
		}
		for _, test := range []struct {
			name  string
			calls []parser.NavigationCall
			rows  int
		}{
			{"duplicate record", []parser.NavigationCall{call, call}, 1},
			{"same line different IDs", []parser.NavigationCall{call, {ID: "second", CallerID: "caller", TargetID: "target", Confidence: "exact", Path: "caller.go", Line: 3}}, 1},
			{"distinct lines", []parser.NavigationCall{call, {ID: "second", CallerID: "caller", TargetID: "target", Confidence: "exact", Path: "caller.go", Line: 4}}, 2},
			{"distinct confidence", []parser.NavigationCall{call, {ID: "second", CallerID: "caller", TargetID: "target", Confidence: "unique-terminal", Path: "caller.go", Line: 3}}, 2},
			{"distinct paths", []parser.NavigationCall{call, {ID: "second", CallerID: "caller", TargetID: "target", Confidence: "exact", Path: "generated.go", Line: 3}}, 2},
			{"distinct ambiguity", []parser.NavigationCall{call, {ID: "second", CallerID: "caller", CandidateTargetIDs: []string{"target"}, Confidence: "exact", Path: "caller.go", Line: 3}}, 2},
			{"duplicate candidate IDs", []parser.NavigationCall{{ID: "candidate", CallerID: "caller", CandidateTargetIDs: []string{"target", "target"}, Confidence: "candidate", Path: "caller.go", Line: 3}}, 1},
		} {
			t.Run(direction+"/"+test.name, func(t *testing.T) {
				var output strings.Builder
				writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: direction, Depth: 1, RootIDs: []string{rootID}}, declarations, test.calls)
				if got := strings.Count(output.String(), "call:"); got != test.rows {
					t.Fatalf("got %d evidence rows, want %d:\n%s", got, test.rows, output.String())
				}
				if strings.Contains(output.String(), "already shown") {
					t.Fatalf("misleading declaration marker:\n%s", output.String())
				}
			})
		}
	}
}

func TestCompactCalleesKeepsDistinctCandidateTargets(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Root", Kind: "func", Path: "flow.go", Start: 1, End: 1},
		{ID: "one", Name: "One", Kind: "func", Path: "flow.go", Start: 2, End: 2},
		{ID: "two", Name: "Two", Kind: "func", Path: "flow.go", Start: 3, End: 3},
	}
	calls := []parser.NavigationCall{{ID: "candidate", CallerID: "root", CandidateTargetIDs: []string{"one", "one", "two"}, Confidence: "candidate", Path: "flow.go", Line: 1}}
	var output strings.Builder
	writeCompactDirectionalQuery(func(line string) bool { fmt.Fprintln(&output, line); return true }, Query{Direction: "callees", Depth: 1, RootIDs: []string{"root"}}, declarations, calls)
	for _, want := range []string{"-> ? func One flow.go:2 call:1 [candidate]", "-> ? func Two flow.go:3 call:1 [candidate]"} {
		if strings.Count(output.String(), want) != 1 {
			t.Fatalf("expected candidate once: %q:\n%s", want, output.String())
		}
	}
}
