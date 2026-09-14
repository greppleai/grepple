package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRelatedOmissionCountsUseStableJSONFields(t *testing.T) {
	result := FileResult{
		Path: "caller.go", Language: "go", Matches: []ResultMatch{}, Segments: []ResultSegment{},
		OmittedRelatedCallers: 2, OmittedRelatedCallees: 1,
		Related: []RelatedSymbol{{Name: "target", OmittedCallers: 4, OmittedCallees: 3}},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"omittedRelatedCallers":2`, `"omittedRelatedCallees":1`,
		`"omittedCallers":4`, `"omittedCallees":3`,
	} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON missing %s: %s", field, encoded)
		}
	}
}

func TestSearchRequestLineRangesUseStableJSONFields(t *testing.T) {
	encoded, err := json.Marshal(SearchRequest{LineRanges: true, EnclosingRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"matchLineRanges":true,"enclosingLineRanges":true}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

func TestResultMatchRangesAreAdditive(t *testing.T) {
	encoded, err := json.Marshal(ResultMatch{Line: 3, EndLine: 5, Text: "if ready {"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"line":3,"endLine":5,"text":"if ready {"}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
	encoded, err = json.Marshal(ResultMatch{Line: 4, StartLine: 3, EndLine: 5, Text: "work()"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"line":4,"startLine":3,"endLine":5,"text":"work()"}` {
		t.Fatalf("unexpected enclosing JSON: %s", encoded)
	}
	encoded, err = json.Marshal(ResultMatch{Line: 7, Text: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "startLine") || strings.Contains(string(encoded), "endLine") {
		t.Fatalf("zero endLine should remain wire-compatible: %s", encoded)
	}
}

func TestSourceAnalysisUsesAdditiveStableJSONFields(t *testing.T) {
	response := SearchResponse{
		Results:        []FileResult{{Path: "broken.go", Language: "go", StructureStatus: "recovered", Matches: []ResultMatch{}, Segments: []ResultSegment{}}},
		SourceAnalysis: &SourceAnalysis{Returned: 1, Recovered: 1},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"structureStatus":"recovered"`, `"sourceAnalysis":{"returned":1`, `"recovered":1`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON missing %s: %s", field, encoded)
		}
	}
}
