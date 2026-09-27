package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeBoundaryPolicyRejectsInvalidOrTrailingInput(t *testing.T) {
	for _, content := range []string{
		`{"schema":"unsupported"}`,
		`{"unknown":true}`,
		`{"schema":"grepple-boundary-policy-v1"} {}`,
	} {
		if _, err := DecodeBoundaryPolicy([]byte(content)); err == nil {
			t.Fatalf("accepted invalid policy %q", content)
		}
	}
	if _, err := DecodeBoundaryPolicy([]byte(`{"schema":"grepple-boundary-policy-v1"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeSourcesPreservesGraphQueryAndCompleteness(t *testing.T) {
	request := AnalysisRequest{Operation: AnalysisGraph, Graph: &GraphQueryRequest{Direction: "callees", Depth: 1, Symbol: "Caller"}}
	sources := []AnalysisSource{{Path: "pkg/main.go", Content: []byte("package pkg\nfunc Target() {}\nfunc Caller() { Target() }\n")}}
	content, complete, err := AnalyzeSources(request, sources, nil, "")
	if err != nil || !complete {
		t.Fatalf("analysis failed: complete=%t err=%v", complete, err)
	}
	var report struct {
		Query *struct {
			RootIDs []string `json:"rootIds"`
		} `json:"query"`
		Calls []json.RawMessage `json:"calls"`
	}
	if err := json.Unmarshal(content, &report); err != nil || report.Query == nil || len(report.Query.RootIDs) != 1 || len(report.Calls) != 1 {
		t.Fatalf("graph result=%s err=%v", content, err)
	}
	request.MaxFiles = 1
	sources = append(sources, AnalysisSource{Path: "pkg/other.go", Content: []byte("package pkg\nfunc Other() {}\n")})
	_, complete, err = AnalyzeSources(request, sources, nil, "")
	if err != nil || complete {
		t.Fatalf("truncation must be incomplete: complete=%t err=%v", complete, err)
	}
	request.Operation = "invalid"
	if _, _, err := AnalyzeSources(request, sources, nil, ""); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("invalid operation should fail: %v", err)
	}
}
