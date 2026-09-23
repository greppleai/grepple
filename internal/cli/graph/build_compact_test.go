package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestCompactBuildGroupsFilesAndRetainsDistinctCallSites(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "helper", Name: "Helper", Kind: "func", Path: "pkg/helper.go", Start: 2, End: 3, Visibility: "public"},
		{ID: "root", Name: "Root", Kind: "func", Path: "pkg/main.go", Start: 5, End: 9, Visibility: "public", Entrypoint: "main"},
		{ID: "local", Name: "Local", Kind: "func", Path: "pkg/main.go", Start: 11, End: 12, Visibility: "non-public"},
	}
	calls := []parser.NavigationCall{
		{ID: "b", CallerID: "root", TargetID: "helper", Path: "pkg/main.go", Line: 7, Confidence: "exact"},
		{ID: "a", CallerID: "root", TargetID: "local", Path: "pkg/main.go", Line: 6, Confidence: "unique-terminal"},
		{ID: "c", CallerID: "root", TargetID: "helper", Path: "pkg/main.go", Line: 8, Confidence: "exact"},
	}
	var output strings.Builder
	writeCompactBuildGraph(func(line string) bool { fmt.Fprintln(&output, line); return true }, declarations, calls)
	want := "pkg/helper.go\n" +
		"  2-3 func Helper visibility=public\n" +
		"pkg/main.go\n" +
		"  5-9 func Root visibility=public entrypoint=main\n" +
		"    -> Local:11-12 call:6 [unique-terminal]\n" +
		"    -> Helper pkg/helper.go:2-3 call:7 [exact]\n" +
		"    -> Helper pkg/helper.go:2-3 call:8 [exact]\n" +
		"  11-12 func Local visibility=non-public\n"
	if got := output.String(); got != want {
		t.Fatalf("compact build:\n%s\nwant:\n%s", got, want)
	}
}

func TestCompactBuildDisclosesCandidatesAndMissingTargets(t *testing.T) {
	declarations := []parser.NavigationDeclaration{
		{ID: "root", Name: "Root", Kind: "func", Path: "main.go", Start: 2, End: 3},
		{ID: "one", Name: "One", Kind: "func", Path: "main.go", Start: 5, End: 5},
		{ID: "two", Name: "Two", Kind: "func", Path: "elsewhere.go", Start: 2, End: 2},
		{ID: "three", Name: "Three", Kind: "func", Path: "elsewhere.go", Start: 4, End: 4},
		{ID: "four", Name: "Four", Kind: "func", Path: "elsewhere.go", Start: 6, End: 6},
	}
	calls := []parser.NavigationCall{
		{ID: "candidate", CallerID: "root", CandidateTargetIDs: []string{"one", "two", "three", "four"}, Path: "main.go", Line: 2, Confidence: "candidate"},
		{ID: "missing", CallerID: "root", TargetID: "unavailable", Name: "Remote", Path: "main.go", Line: 3, Confidence: "import-resolved"},
	}
	var output strings.Builder
	writeCompactBuildGraph(func(line string) bool { fmt.Fprintln(&output, line); return true }, declarations, calls)
	got := output.String()
	for _, want := range []string{
		"    -> ? One:5, Two elsewhere.go:2, Three elsewhere.go:4, +1 call:2 [candidate]\n",
		"    -> ? Remote (target unavailable) call:3 [import-resolved]\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q from compact build:\n%s", want, got)
		}
	}
}
