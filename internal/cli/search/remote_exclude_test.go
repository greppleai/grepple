package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/search"
)

func TestSearchRemoteSendsAndAppliesRepoExclusion(t *testing.T) {
	var received wire.SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(response).Encode(wire.SearchResponse{Results: []wire.FileResult{
			{Repo: "owner/current", Path: "current.go"},
			{Repo: "owner/other", Path: "other.go"},
		}})
	}))
	defer server.Close()

	options := &cliOptions{Params: search.Params{
		Query:       "needle",
		Regex:       false,
		ExcludeRepo: []string{"owner/current"},
	}}
	results, err := runTestRemote(options, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	params, err := search.ResolveRequest(received)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.ExcludeRepo) != 1 || params.ExcludeRepo[0] != "owner/current" {
		t.Fatalf("excludeRepo=%v", params.ExcludeRepo)
	}
	if len(results) != 1 || results[0].Repo != "owner/other" {
		t.Fatalf("results=%#v", results)
	}
}
