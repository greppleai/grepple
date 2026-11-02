package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	searchengine "github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

// The fixture applies the offset before the server's per-page limit. An
// oversized prefix request cannot bypass that cap.
func TestRemoteOffsetFilesWithMatches(t *testing.T) {
	chdirTemp(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	const corpusSize = 250
	for _, tc := range []struct{ skip, limit, want int }{
		{0, 100, 100}, {100, 100, 100}, {200, 100, 50},
		{75, 50, 50}, {250, 100, 0}, {100, 0, 100}, {100, 200, 100},
	} {
		t.Run(fmt.Sprintf("skip_%d_limit_%d", tc.skip, tc.limit), func(t *testing.T) {
			var requests []wire.SearchRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/public/search" {
					t.Errorf("endpoint = %s, want legacy search", r.URL.Path)
				}
				var request wire.SearchRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests = append(requests, request)
				params, err := searchengine.ResolveRequest(request)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				limit := params.Limit
				if limit == 0 || limit > searchengine.MaxPageLimit {
					limit = searchengine.MaxPageLimit
				}
				results := []wire.FileResult{}
				for i := params.Skip; i < corpusSize && i < params.Skip+limit; i++ {
					results = append(results, wire.FileResult{Repo: "owner/remote", Path: fmt.Sprintf("file-%03d.txt", i)})
				}
				_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: results})
			}))
			defer server.Close()

			var stdout, stderr bytes.Buffer
			exitCode := 0
			application := cliruntime.Environment{
				Output: &stdout, ErrorOutput: &stderr,
				Exit: func(code int) { exitCode = code },
			}
			err := New(application).Run([]string{"--remote-only", "--server", server.URL,
				"-F", "needle", "--files-with-matches", "--json", "--limit", strconv.Itoa(tc.limit), "--skip", strconv.Itoa(tc.skip)})
			if err != nil {
				t.Fatal(err)
			}
			var output struct {
				Files []struct {
					Path string `json:"path"`
				} `json:"files"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
				t.Fatalf("decode output: %v; stdout=%s", err, &stdout)
			}
			if len(output.Files) != tc.want {
				t.Errorf("returned %d files, want %d", len(output.Files), tc.want)
			}
			for i, file := range output.Files {
				if want := fmt.Sprintf("file-%03d.txt", tc.skip+i); file.Path != want {
					t.Errorf("file %d = %q, want %q", i, file.Path, want)
				}
			}
			wantExit := 0
			if tc.want == 0 {
				wantExit = 1
			}
			if exitCode != wantExit {
				t.Errorf("exit = %d, want %d", exitCode, wantExit)
			}
			wantLimit := tc.limit
			if wantLimit == 0 || wantLimit > searchengine.MaxPageLimit {
				wantLimit = searchengine.MaxPageLimit
				if !strings.Contains(stderr.String(), "server pages are capped at 100 files") {
					t.Errorf("missing cap warning: %s", &stderr)
				}
			} else if stderr.Len() != 0 {
				t.Errorf("unexpected stderr: %s", &stderr)
			}
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			request := requests[0]
			if request.Skip == nil || *request.Skip != tc.skip || request.Limit == nil || *request.Limit != wantLimit {
				t.Errorf("request window: skip=%v limit=%v, want %d/%d", request.Skip, request.Limit, tc.skip, wantLimit)
			}
			if request.Query == nil || *request.Query != "needle" || request.Regex == nil || *request.Regex || !request.SkipSegments {
				t.Errorf("literal file-listing request not preserved: %#v", request)
			}
		})
	}
}
