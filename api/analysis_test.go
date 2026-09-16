package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnalysisRequestWireContract(t *testing.T) {
	request := AnalysisRequest{Operation: AnalysisGraph, Repository: "owner/repo@tag~v1.2.3", Paths: []string{"internal/**"}, MaxFiles: 20, ProductionOnly: true, Graph: &GraphQueryRequest{Direction: "impact", Depth: 2, Symbol: "Run", Languages: []string{"go"}}}
	content, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"operation":"graph"`, `"repository":"owner/repo@tag~v1.2.3"`, `"productionOnly":true`, `"direction":"impact"`} {
		if !strings.Contains(string(content), fragment) {
			t.Fatalf("wire payload %s lacks %s", content, fragment)
		}
	}
	var decoded AnalysisRequest
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Repository != request.Repository || decoded.Graph == nil || decoded.Graph.Symbol != "Run" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestAnalysisResponseEmbedsVersionedResult(t *testing.T) {
	response := AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: AnalysisArchitecture, Repository: "owner/repo", Commit: "abc", Found: true, Complete: true, Result: json.RawMessage(`{"schema":"grepple-directory-architecture-v4"}`)}
	content, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"result":{"schema":"grepple-directory-architecture-v4"}`) {
		t.Fatalf("response=%s", content)
	}
}
