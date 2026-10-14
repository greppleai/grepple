package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestScopeAnnotationArgumentsAndLegacyDefaults(t *testing.T) {
	for _, args := range [][]string{{"--annotate-scopes", "work"}, {"--line-only", "--annotate-scopes", "work"}} {
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Errorf("accepted removed flag: %v", args)
		}
	}
	for _, extra := range [][]string{{}, {"--json"}, {"--json-matches"}} {
		args := append([]string{"--line-only"}, extra...)
		args = append(args, "work")
		options, _, _, err := parseTestSearchArgs(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !options.Params.AnnotateScopes || !options.Params.LineRanges || !options.Params.SkipSegments || !searchRequestFromParams(options.Params).AnnotateScopes {
			t.Fatalf("default lost annotation request: %v: %+v", args, options.Params)
		}
	}
	for _, args := range [][]string{
		{"--line-only", "--no-annotate-scopes", "work"},
		{"--line-only", "--enclosing", "work"},
		{"--line-only", "-C", "1", "work"},
		{"--line-only", "-o", "work"},
		{"--line-only", "--count", "work"},
		{"--line-only", "--files-with-matches", "work"},
		{"--line-only", "--outline", "a.go"},
		{"--line-only", "--at", "a.go:2-3"},
		{"work"},
	} {
		options, _, _, err := parseTestSearchArgs(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if options.Params.AnnotateScopes || searchRequestFromParams(options.Params).AnnotateScopes {
			t.Fatalf("unexpected annotations in non-default mode: %v", args)
		}
	}
}

func TestRemoteScopeCapabilityLegacyAndCursorPages(t *testing.T) {
	for _, route := range []string{"/public/search", "/public/search/pages"} {
		for _, version := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", route, version), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != route {
						t.Errorf("route=%s, want %s", r.URL.Path, route)
					}
					if route == "/public/search/pages" {
						var request apiclient.SearchPageRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						if request.Search == nil || !request.Search.AnnotateScopes {
							t.Errorf("missing page request opt-in: %+v", request)
						}
						_ = json.NewEncoder(w).Encode(apiclient.SearchPage{Results: []wire.FileResult{}, Complete: true, PageID: fixtureCursor("A"), ScopeAnnotationsVersion: version})
					} else {
						var request wire.SearchRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						if !request.AnnotateScopes {
							t.Errorf("missing legacy request opt-in: %+v", request)
						}
						_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: []wire.FileResult{{Path: "owner/repo/a.go", Matches: []wire.ResultMatch{{Line: 2, Text: "work", Nearest: &parser.ScopeLineRange{Kind: "func", StartLine: 1, EndLine: 3}}}}}, ScopeAnnotationsVersion: version})
					}
				}))
				defer server.Close()
				p := search.Params{Query: "work", Regex: true, LineRanges: true, AnnotateScopes: true, SkipSegments: true, Sort: search.ResultSortPath}
				if route == "/public/search" {
					p.Sort = search.ResultSortMatches
				} // force legacy endpoint
				got, err := runTestRemote(&Options{Params: p}, server.URL)
				if version != wire.ScopeAnnotationsVersion {
					if err == nil || !strings.Contains(err.Error(), "scopeAnnotationsVersion") || len(got) != 0 {
						t.Fatalf("old/mismatched version %d rendered: %+v, %v", version, got, err)
					}
				} else if err != nil {
					t.Fatalf("capable router rejected: %v", err)
				}
			})
		}
	}
	// Unannotated clients retain old-server behavior, including absent markers.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: []wire.FileResult{}})
	}))
	defer server.Close()
	if _, err := runTestRemote(&Options{Params: search.Params{Query: "work", Sort: search.ResultSortMatches}}, server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestWireScopeAnnotationRoundTrip(t *testing.T) {
	in := wire.SearchResponse{ScopeAnnotationsVersion: wire.ScopeAnnotationsVersion, Results: []wire.FileResult{{Path: "a.go", Matches: []wire.ResultMatch{{Line: 2, StartLine: 1, EndLine: 4, Text: "work()", Nearest: &wire.ScopeLineRange{Kind: "if", StartLine: 2, EndLine: 3}, Scope: &wire.ScopeLineRange{Kind: "func", StartLine: 1, EndLine: 4}}}}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out wire.SearchResponse
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ScopeAnnotationsVersion != 1 || len(out.Results) != 1 || out.Results[0].Matches[0].Scope == nil || out.Results[0].Matches[0].Scope.Kind != "func" || out.Results[0].Matches[0].StartLine != 1 {
		t.Fatalf("round trip: %s", raw)
	}
	legacy, _ := json.Marshal(wire.SearchResponse{Results: []wire.FileResult{}})
	if strings.Contains(string(legacy), "scopeAnnotationsVersion") {
		t.Fatalf("legacy response grew marker: %s", legacy)
	}
}
