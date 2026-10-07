package search

import (
	"encoding/json"
	"github.com/greppleai/grepple/internal/apiclient"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestSearchRemoteSendsAndAppliesRepoExclusion(t *testing.T) {
	var received wire.SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/public/search/pages" {
			t.Errorf("unexpected endpoint %s", request.URL.Path)
		}
		var body apiclient.SearchPageRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		received = *body.Search
		_ = json.NewEncoder(response).Encode(apiclient.SearchPage{Complete: true, PageID: strings.Repeat("A", 43) + "." + strings.Repeat("B", 43), ExpiresAt: time.Now().Add(time.Minute), Results: []wire.FileResult{
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
