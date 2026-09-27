package graph

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/parser"
)

func TestRemoteCommandPreservesExactRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input wire.AnalysisRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.Operation != wire.AnalysisGraph || input.Repository != "owner/repo@tag~v1" {
			t.Fatalf("request=%+v", input)
		}
		result, _ := json.Marshal(map[string]any{"schema": Schema})
		_ = json.NewEncoder(writer).Encode(wire.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	var output bytes.Buffer
	if err := runBuild(cliruntime.Environment{Output: &output}, []string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"repository": "owner/repo@tag~v1"`) {
		t.Fatalf("output=%s", output.String())
	}
}

func TestRemoteCalleesRendersSignaturesWithoutFetchingSource(t *testing.T) {
	graph := Output{
		Schema: Schema,
		Query:  &Query{Direction: "callees", Depth: 1, RootIDs: []string{"root"}},
		Declarations: []parser.NavigationDeclaration{
			{ID: "root", Name: "Start", Kind: "func", Language: "go", Path: "start.go", Start: 2, End: 4, Signature: "func Start(input Input) Output"},
			{ID: "target", Name: "Load", Kind: "func", Language: "go", Path: "load.go", Start: 5, End: 8, Signature: "func Load(input Input) Output"},
		},
		Calls: []parser.NavigationCall{{ID: "call", CallerID: "root", TargetID: "target", Confidence: "exact", Path: "start.go", Line: 3}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input wire.AnalysisRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.Graph == nil || input.Graph.Direction != "callees" {
			t.Errorf("remote query=%+v err=%v", input, err)
			return
		}
		result, err := json.Marshal(graph)
		if err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(writer).Encode(wire.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	var output bytes.Buffer
	command := New(cliruntime.Environment{Output: &output})
	if err := command.Run([]string{"callees", "--symbol", "Start", "--server", server.URL, "--repo", "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go func Start(input Input) Output @ start.go:2-4", "-> func Load(input Input) Output @ load.go:5-8 call:3 [exact]"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("remote signature output missing %q:\n%s", want, output.String())
		}
	}
}
