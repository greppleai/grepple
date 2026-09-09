package cli

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCountByRepoRendersSortedTable verifies that --count-by-repo issues a compact
// unbounded CountByRepo probe and renders the per-repo tallies (hottest first)
// plus a total.
func TestCountByRepoRendersSortedTable(t *testing.T) {
	chdirTemp(t) // empty dir => no local matches, only the remote counts

	var request api.SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(api.SearchResponse{RepoCounts: []api.RepoCount{
			{Repo: "owner/alpha", Files: 2, Matches: 3},
			{Repo: "owner/beta", Files: 5, Matches: 40},
		}})
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runSearch([]string{"--server", server.URL, "--count-by-repo", "ping"}); err != nil {
			t.Fatal(err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 2 repo rows + total, got %d: %q", len(lines), out)
	}
	// beta (40 matches) is hotter, so it comes first.
	if !strings.HasPrefix(lines[0], "owner/beta\t") {
		t.Fatalf("expected owner/beta first, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "owner/alpha\t") {
		t.Fatalf("expected owner/alpha second, got %q", lines[1])
	}
	if lines[2] != "total\t7 files\t43 matches" {
		t.Fatalf("unexpected total row: %q", lines[2])
	}

	// The probe must be a CountByRepo request with no paging window.
	if !request.CountByRepo {
		t.Fatal("request should set CountByRepo")
	}
	if request.Limit != nil {
		t.Fatalf("counts must not send a --limit window, got %v", *request.Limit)
	}
	if request.MaxFiles != nil {
		t.Fatalf("counts must not send a --max-files window, got %v", *request.MaxFiles)
	}
}

// TestRemoteTruncatedPrintsNarrowingHint verifies the CLI surfaces the index
// truncation flag so an agent knows to narrow rather than trust a partial set.
func TestRemoteTruncatedPrintsNarrowingHint(t *testing.T) {
	chdirTemp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.SearchResponse{
			Results:   []api.FileResult{{Repo: "owner/a", Path: "owner/a/x.go", Matches: []api.ResultMatch{{Line: 1, Text: "ping"}}}},
			Truncated: true,
		})
	}))
	defer server.Close()

	stderr := captureStderr(t, func() {
		if err := runSearch([]string{"--server", server.URL, "--files-with-matches", "ping"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stderr, "truncated") {
		t.Fatalf("expected a truncation/narrowing hint on stderr, got %q", stderr)
	}
}
