package search

import (
	"encoding/json"
	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/wire"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixtureCursor(letter string) string {
	return strings.Repeat("S", 43) + "." + strings.Repeat(letter, 43)
}
func TestCursorRemoteOnlyPagesReplayAndNoLocalMixing(t *testing.T) {
	chdirTemp(t)
	writeGraphSource(t, ".", "local.go", "package local\nfunc Needle(){}\n")
	var bodies []apiclient.SearchPageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/search/pages" {
			t.Errorf("legacy endpoint called: %s", r.URL.Path)
		}
		var body apiclient.SearchPageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		page := apiclient.SearchPage{Results: []wire.FileResult{}, PageID: fixtureCursor("A"), Cursor: fixtureCursor("B"), ExpiresAt: time.Now().Add(time.Minute)}
		if body.Cursor != "" {
			page.PageID = fixtureCursor("B")
			page.Cursor = ""
			page.Complete = true
			for i := 0; i < 3; i++ {
				page.Results = append(page.Results, wire.FileResult{Path: "remote.go", Repo: "owner/remote", Matches: []wire.ResultMatch{{Line: i + 1, Text: "Needle"}}})
			}
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	first := captureStdout(t, func() {
		if err := runTestSearch([]string{"--remote-only", "--server", server.URL, "-F", "--json", "Needle"}); err != nil {
			t.Fatal(err)
		}
	})
	assertCursorCommand(t, first, server.URL)
	second := captureStdout(t, func() {
		if err := runTestSearch([]string{"--cursor", fixtureCursor("B"), "--server", server.URL, "--limit", "1", "--json", "Needle"}); err != nil {
			t.Fatal(err)
		}
	})
	assertCursorReplayFiles(t, second)
	if len(bodies) != 2 || bodies[0].Search == nil || bodies[1].Search != nil || bodies[1].Cursor != fixtureCursor("B") {
		t.Fatalf("invalid start/continuation protocol: %#v", bodies)
	}
}
func TestCursorArgumentsRejectLegacyModes(t *testing.T) {
	for _, flag := range []string{"--local", "--files", "--count", "--count-summary", "--outline"} {
		if err := validateCursorArguments(&Args{Cursor: fixtureCursor("B"), Sort: "path", Local: flag == "--local", Files: flag == "--files", Count: flag == "--count", CountSummary: flag == "--count-summary", Outline: flag == "--outline"}); err == nil {
			t.Fatalf("accepted %s", flag)
		}
	}
}
