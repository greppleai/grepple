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

func TestSearchRequestLineRangesUsesStableJSONField(t *testing.T) {
	encoded, err := json.Marshal(SearchRequest{LineRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"matchLineRanges":true}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

func TestResultMatchEndLineIsAdditive(t *testing.T) {
	encoded, err := json.Marshal(ResultMatch{Line: 3, EndLine: 5, Text: "if ready {"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"line":3,"endLine":5,"text":"if ready {"}` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
	encoded, err = json.Marshal(ResultMatch{Line: 7, Text: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "endLine") {
		t.Fatalf("zero endLine should remain wire-compatible: %s", encoded)
	}
}
