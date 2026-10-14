package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestMixedSearchDoesNotRenderLocalRowsIfRemoteLacksCapability(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "local.go", "package p\nfunc run() {\n  needle()\n}\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/search" {
			t.Errorf("mixed search used %s", r.URL.Path)
		}
		var req wire.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if !req.AnnotateScopes {
			t.Error("mixed remote request lacks opt-in")
		}
		_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: []wire.FileResult{}})
	}))
	defer server.Close()
	var runErr error
	output := captureStdout(t, func() {
		runErr = runTestSearch([]string{"--server", server.URL, "--line-only", "needle"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "scopeAnnotationsVersion") || output != "" {
		t.Fatalf("unsafe partial rendering: stdout=%q err=%v", output, runErr)
	}
}

func TestScopeAnnotationCursorContinuationKeepsOptIn(t *testing.T) {
	opts := &Options{Params: search.Params{Query: "needle", Regex: true, LineRanges: true, AnnotateScopes: true, Limit: 20}, CursorPage: &apiclient.SearchPage{Cursor: fixtureCursor("B")}, RemoteServer: "https://example.test"}
	command := searchNextCommand(testCommandContext(), opts, 20, true)
	for _, token := range []string{"--remote-only", "--cursor", "--line-only", "--json"} {
		if !strings.Contains(command, token) {
			t.Errorf("continuation lost %s: %q", token, command)
		}
	}
	if strings.Contains(command, "--annotate-scopes") || strings.Contains(command, "--no-annotate-scopes") {
		t.Errorf("default continuation should need no scope toggle: %q", command)
	}
	plain := &Options{Params: search.Params{Query: "needle", Regex: true, LineRanges: true, Limit: 20}, CursorPage: &apiclient.SearchPage{Cursor: fixtureCursor("B")}, RemoteServer: "https://example.test"}
	if command := searchNextCommand(testCommandContext(), plain, 20, true); !strings.Contains(command, "--line-only --no-annotate-scopes") {
		t.Errorf("plain cursor continuation lost opt-out: %q", command)
	}
	old := searchNextCommand(testCommandContext(), &Options{Params: search.Params{Query: "needle", Limit: 20}}, 20, false)
	if strings.Contains(old, "--line-only") {
		t.Errorf("legacy continuation changed: %s", old)
	}
}
