package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestScopeAnnotationsWindowedFilesAndLegacyRanges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("package p\nfunc run() {\n if true {\n  work()\n }\n}\n// work outside\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	params := Params{Query: "work", Regex: true, Root: dir, SkipSegments: true, LineRanges: true, Limit: 1, Skip: 1}
	baseline, err := Files(params, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 1 || baseline[0].File != b || baseline[0].ScopeAnnotations != nil {
		t.Fatalf("baseline window: %+v", baseline)
	}
	params.AnnotateScopes = true
	annotated, err := Files(params, []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(annotated) != 1 || annotated[0].File != b {
		t.Fatalf("selected window: %+v", annotated)
	}
	if !reflect.DeepEqual(annotated[0].MatchRanges, baseline[0].MatchRanges) {
		t.Fatalf("legacy ranges changed: %v vs %v", annotated[0].MatchRanges, baseline[0].MatchRanges)
	}
	if annotated[0].SegmentsReady || len(annotated[0].Segments) > 0 {
		t.Fatal("compact mode built segments")
	}
	result := ToResult(annotated[0], nil, 0, 0)
	if len(result.Matches) != 2 || result.Matches[0].Line != 4 || result.Matches[0].Nearest == nil || *result.Matches[0].Nearest != (parser.ScopeLineRange{Kind: "if", StartLine: 3, EndLine: 5}) || result.Matches[0].Scope == nil || *result.Matches[0].Scope != (parser.ScopeLineRange{Kind: "func", StartLine: 2, EndLine: 6}) {
		t.Fatalf("body roles: %+v", result.Matches)
	}
	if result.Matches[1].Line != 7 || result.Matches[1].Nearest != nil || result.Matches[1].Scope != nil {
		t.Fatalf("outside comment invented scope: %+v", result.Matches[1])
	}
	for i, match := range result.Matches {
		old := ToResult(baseline[0], nil, 0, 0).Matches[i]
		if match.Line != old.Line || match.Text != old.Text || match.StartLine != old.StartLine || match.EndLine != old.EndLine {
			t.Fatalf("legacy range/text changed: %+v vs %+v", match, old)
		}
	}
}

func TestScopeAnnotationsSortStdinAndWireRoundTrip(t *testing.T) {
	params := Params{Query: "work", Regex: true, SkipSegments: true, LineRanges: true, AnnotateScopes: true}
	stdin, err := Content(params, StdinPath, []byte("func run() { work() }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if stdin == nil || stdin.Language != "text" || len(stdin.ScopeAnnotations) != 0 {
		t.Fatalf("stdin classified as code: %+v", stdin)
	}
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package p\nfunc run(){work()}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	params.Root = root
	params.Sort = ResultSortMatches
	selected, err := Files(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].DisplayPath != "a.go" || selected[1].DisplayPath != "b.go" {
		t.Fatalf("ranked paths: %+v", selected)
	}
	req := RequestFromParams(params)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"annotateScopes":true`) {
		t.Fatalf("missing opt-in: %s", raw)
	}
	var decoded Request
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveRequest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.AnnotateScopes || !resolved.LineRanges {
		t.Fatalf("wire request lost opt-in: %+v", resolved)
	}
	raw, err = json.Marshal(ToResult(selected[0], nil, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	var result FileResult
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Nearest == nil || result.Matches[0].Scope == nil || result.Matches[0].Scope.Kind != "func" {
		t.Fatalf("wire role round trip: %s", raw)
	}
	// Legacy zero request/match must not acquire even a null annotation key.
	legacy, _ := json.Marshal(RequestFromParams(Params{Query: "work"}))
	oldMatch, _ := json.Marshal(ResultMatch{Line: 1, Text: "work"})
	if strings.Contains(string(legacy), "annotateScopes") || strings.Contains(string(oldMatch), "nearest") || strings.Contains(string(oldMatch), "scope") {
		t.Fatalf("legacy JSON changed: %s / %s", legacy, oldMatch)
	}
}
