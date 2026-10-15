package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestCompactQueryDiamondRecursionRootsAndOrdering(t *testing.T) {
	declarations := []parser.NavigationDeclaration{}
	for i, id := range []string{"root", "left", "right", "leaf"} {
		declarations = append(declarations, parser.NavigationDeclaration{ID: id, Name: id, Kind: "func", Language: "go", Path: "flow.go", Start: i + 1, End: i + 1})
	}
	calls := []parser.NavigationCall{}
	for i, pair := range [][2]string{{"root", "left"}, {"root", "right"}, {"left", "leaf"}, {"right", "leaf"}, {"leaf", "leaf"}, {"leaf", "root"}} {
		calls = append(calls, parser.NavigationCall{ID: fmt.Sprint(i), CallerID: pair[0], TargetID: pair[1], Path: "flow.go", Line: i + 10, Confidence: "exact"})
	}
	query := Query{Direction: "callees", Depth: 4, RootIDs: []string{"root", "right"}}
	render := func(input []parser.NavigationCall, limit int) string {
		var output strings.Builder
		writeCompactDirectionalQuery(func(line string) bool {
			if limit > 0 && output.Len()+len(line)+1 > limit {
				return false
			}
			fmt.Fprintln(&output, line)
			return true
		}, query, declarations, input)
		return output.String()
	}
	baseline := render(calls, 0)
	// Every unique edge is present in each independent root section, including
	// the diamond's second incoming edge and the recursive/back edges.
	for i := range calls {
		if got := strings.Count(baseline, fmt.Sprintf("call:%d [exact]", i+10)); got != 2 {
			t.Fatalf("edge %d: got %d, want 2:\n%s", i, got, baseline)
		}
	}
	reversed := append([]parser.NavigationCall(nil), calls...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if got := render(reversed, 0); got != baseline {
		t.Fatalf("input-dependent ordering:\n%s\n%s", baseline, got)
	}
	duplicates := append(append([]parser.NavigationCall(nil), calls...), calls...)
	if got := render(duplicates, len(baseline)); got != baseline {
		t.Fatalf("duplicate evidence consumed output budget:\n%s\n%s", baseline, got)
	}
	if got := render(duplicates, 80); len(got) > 80 || got != render(calls, 80) {
		t.Fatalf("bounded output differs: %q", got)
	}
}
