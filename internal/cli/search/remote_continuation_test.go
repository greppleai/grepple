package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	engine "github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestLegacyRemoteContinuationReplaysWindowAndMode(t *testing.T) {
	chdirTemp(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	writeGraphSource(t, ".", "local.txt", "needle\n")
	for _, filesOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("files_%v", filesOnly), func(t *testing.T) {
			var requests []engine.Params
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/public/search" {
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
				var request wire.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				params, err := engine.ResolveRequest(request)
				if err != nil {
					t.Error(err)
					return
				}
				requests = append(requests, params)
				if params.Query != "needle" || params.Regex || params.Sort != "matches" || !reflect.DeepEqual(params.Repo, []string{"owner/remote"}) || !reflect.DeepEqual(params.ExcludeRepo, []string{"owner/excluded"}) {
					t.Errorf("lost search options: %+v", params)
				}
				if filesOnly && !params.SkipSegments {
					t.Error("file listing lost SkipSegments")
				}
				limit := params.Limit
				if limit == 0 || limit > engine.MaxPageLimit {
					limit = engine.MaxPageLimit
				}
				results := []wire.FileResult{}
				for i := params.Skip; i < 250 && i < params.Skip+limit; i++ {
					results = append(results, wire.FileResult{Repo: "owner/remote", Path: fmt.Sprintf("file-%03d.txt", i), Matches: []wire.ResultMatch{{Line: 1, Text: "needle"}}})
				}
				_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: results})
			}))
			defer server.Close()
			var stdout, stderr bytes.Buffer
			exit := 0
			app := cliruntime.Environment{Output: &stdout, ErrorOutput: &stderr, Exit: func(code int) { exit = code }}
			args := []string{"--remote-only", "--server", server.URL, "-F", "needle", "--sort", "matches", "--repo", "owner/remote", "--exclude-repo", "owner/excluded", "--skip", "100", "--limit", "100", "--json"}
			if filesOnly {
				args = append(args, "--files-with-matches")
			}
			for page, want := range []int{100, 50, 0} {
				stdout.Reset()
				stderr.Reset()
				exit = 0
				options, explicit, remote, err := parseSearchArgs(app, args)
				if err != nil {
					t.Fatal(err)
				}
				if err := executeSearch(app, options, explicit, remote); err != nil {
					t.Fatal(err)
				}
				metadata := options.ResultMetadata
				wantSkip := []int{100, 200, 250}[page]
				if metadata == nil || metadata.Scope.Mode != "remote" || metadata.Page.Skip != wantSkip || metadata.Page.Limit != 100 || metadata.Page.Returned != want || metadata.Page.Total != nil || metadata.Page.Complete {
					t.Fatalf("wrong remote metadata: %+v", metadata)
				}
				var output struct {
					Files   []wire.FileResult `json:"files"`
					Results []wire.FileResult `json:"results"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
					t.Fatal(err)
				}
				if !filesOnly {
					output.Files = output.Results
				}
				if len(output.Files) != want {
					t.Fatalf("returned %d files, want %d: %s", len(output.Files), want, &stdout)
				}
				for i, file := range output.Files {
					if file.Path != fmt.Sprintf("file-%03d.txt", wantSkip+i) {
						t.Errorf("wrong file: %s", file.Path)
					}
				}
				if stderr.Len() != 0 {
					t.Errorf("unexpected stderr: %s", &stderr)
				}
				if want == 0 {
					if exit != 1 || metadata.NextCommand != "" {
						t.Fatalf("bad exhaustion: exit=%d next=%s", exit, metadata.NextCommand)
					}
					break
				}
				if exit != 0 {
					t.Fatalf("exit=%d", exit)
				}
				// This fixture deliberately uses shell-safe, whitespace-free tokens, so
				// Fields replays the emitted command without invoking a shell or binary.
				command := strings.Fields(metadata.NextCommand)
				if len(command) < 3 || command[0] != "grepple" || command[1] != "search" {
					t.Fatalf("bad continuation: %q", metadata.NextCommand)
				}
				args = command[2:]
			}
			if len(requests) != 3 {
				t.Fatalf("requests=%d", len(requests))
			}
			for i, request := range requests {
				if request.Skip != []int{100, 200, 250}[i] || request.Limit != 100 {
					t.Errorf("wrong request window: %+v", request)
				}
			}
		})
	}
}

func TestSearchContinuationKeepsCapsAndMixedSource(t *testing.T) {
	options := &Options{Params: engine.Params{Query: "needle", Limit: 25, MaxFiles: 5, BeforeContext: 2, AfterContext: 3, Sort: "matches", Repo: []string{"owner/repo"}, ExcludeRepo: []string{"owner/excluded"}}, RemoteServer: "https://example.test", FilesWithMatches: true}
	command := searchNextCommand(testCommandContext(), options, 10, true)
	for _, part := range []string{"--remote --server https://example.test", "--files-with-matches", "--max-files 5", "--before-context 2", "--after-context 3", "--sort matches", "--repo owner/repo", "--exclude-repo owner/excluded", "--skip 10"} {
		if !strings.Contains(command, part) {
			t.Errorf("lost %s in %s", part, command)
		}
	}
	if strings.Contains(command, "--remote-only") {
		t.Fatalf("mixed continuation became remote-only: %s", command)
	}
	parsed, server, remote, err := parseTestSearchArgs(strings.Fields(command)[2:])
	if err != nil {
		t.Fatal(err)
	}
	if !remote || parsed.RemoteOnly || server != options.RemoteServer || parsed.Params.MaxFiles != 5 || !parsed.FilesWithMatches {
		t.Fatalf("wrong replay: %+v, server=%s remote=%v", parsed, server, remote)
	}
}
