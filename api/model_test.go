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
