package cli

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns whatever
// was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	fn()
	_ = writer.Close()
	os.Stdout = previous
	return <-done
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns whatever
// was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- string(data)
	}()
	fn()
	_ = writer.Close()
	os.Stderr = previous
	return <-done
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return dir
}

// TestFilesWithMatchesKeepsContentQuery verifies that --files-with-matches runs a
// content search (query preserved, not moved into globs) with segments skipped.
func TestFilesWithMatchesKeepsContentQuery(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"--files-with-matches", "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if options == nil {
		t.Fatal("expected options")
	}
	if !options.FilesWithMatches {
		t.Fatal("FilesWithMatches should be set")
	}
	if options.Params.Files {
		t.Fatal("Files (filename glob) must stay false for a content search")
	}
	if options.Params.Query != "ping" || len(options.Params.Globs) != 0 {
		t.Fatalf("query should stay in Query, not Globs: %#v", options.Params)
	}
	if !options.Params.SkipSegments {
		t.Fatal("SkipSegments should be set for paths-only output")
	}
}

func TestFilesAndFilesWithMatchesAreMutuallyExclusive(t *testing.T) {
	if _, _, _, err := parseSearchArgs([]string{"--files", "--files-with-matches", "ping"}); err == nil {
		t.Fatal("expected an error when both --files and --files-with-matches are set")
	}
}

// TestFilesWithMatchesPrintsPathsOnly runs an end-to-end search against a mock
// server whose results carry match lines and segments, and asserts the output is
// exactly the file paths (one per line) with none of the match/segment text.
func TestFilesWithMatchesPrintsPathsOnly(t *testing.T) {
	// Run from an empty dir so the local search contributes nothing.
	dir := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var request api.SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(api.SearchResponse{Results: []api.FileResult{
			{Repo: "owner/alpha", Path: "owner/alpha/svc/ping.go",
				Matches:  []api.ResultMatch{{Line: 3, Text: "func ping() {"}},
				Segments: []api.ResultSegment{{Kind: "function", Start: 3, End: 5, Text: "func ping() {}"}}},
			{Repo: "owner/beta", Path: "owner/beta/handler.go",
				Matches: []api.ResultMatch{{Line: 8, Text: "// ping health"}}},
		}})
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runSearch([]string{"--server", server.URL, "--files-with-matches", "ping"}); err != nil {
			t.Fatal(err)
		}
	})

	assertPathsOnlyOutput(t, out)
	assertContentSearchRequest(t, request)
}

// assertPathsOnlyOutput checks the output is exactly the two file paths, one
// per line, with none of the match/segment text leaking through.
func assertPathsOnlyOutput(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"owner/alpha/svc/ping.go", "owner/beta/handler.go"}
	if len(lines) != len(want) {
		t.Fatalf("expected %d path lines, got %d: %q", len(want), len(lines), out)
	}
	got := map[string]bool{}
	for _, l := range lines {
		got[l] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Fatalf("missing path %q in output: %q", w, out)
		}
	}
	for _, leak := range []string{"func ping()", "ping health", "function"} {
		if strings.Contains(out, leak) {
			t.Fatalf("paths-only output leaked content %q: %q", leak, out)
		}
	}
}

// assertContentSearchRequest checks the outgoing request was a content search
// with segments skipped (not a filename glob).
func assertContentSearchRequest(t *testing.T, request api.SearchRequest) {
	t.Helper()
	if request.Files {
		t.Fatal("request.Files should be false (content search, not filename glob)")
	}
	if !request.SkipSegments {
		t.Fatal("request.SkipSegments should be true")
	}
	if request.Query == nil || *request.Query != "ping" {
		t.Fatalf("request query = %v, want ping", request.Query)
	}
}

func TestParseDefaultLimit(t *testing.T) {
	opts, _, _, err := parseSearchArgs([]string{"needle"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Params.Limit != DefaultResultLimit {
		t.Fatalf("default limit = %d, want %d", opts.Params.Limit, DefaultResultLimit)
	}
	zero, _, _, err := parseSearchArgs([]string{"--limit", "0", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if zero.Params.Limit != 0 {
		t.Fatalf("--limit 0 = %d, want 0 (unbounded)", zero.Params.Limit)
	}
}

// TestLocalFilesWithMatchesIsContentSearch guards the fix that --files-with-matches
// is a content search locally: it must list only files whose contents match, not
// every file in the tree.
func TestLocalFilesWithMatchesIsContentSearch(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(dir+"/has.txt", []byte("a ping here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/none.txt", []byte("nothing to see\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runSearch([]string{"--local", "--files-with-matches", "ping"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "has.txt") {
		t.Fatalf("expected has.txt (contains ping): %q", out)
	}
	if strings.Contains(out, "none.txt") {
		t.Fatalf("listed a non-matching file: %q", out)
	}
}

// TestDefaultLimitCapsAndHints creates more than the default number of matching
// files and asserts the output is capped and a hint is printed to stderr.
func TestDefaultLimitCapsAndHints(t *testing.T) {
	dir := chdirTemp(t)
	for i := 0; i < DefaultResultLimit+5; i++ {
		name := dir + "/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".txt"
		if err := os.WriteFile(name, []byte("ping\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out string
	stderr := captureStderr(t, func() {
		out = captureStdout(t, func() {
			if err := runSearch([]string{"--local", "--files-with-matches", "ping"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != DefaultResultLimit {
		t.Fatalf("expected %d paths, got %d", DefaultResultLimit, len(lines))
	}
	if !strings.Contains(stderr, "default --limit") {
		t.Fatalf("expected default-limit hint on stderr, got %q", stderr)
	}

	// --limit 0 disables the cap and the hint.
	var outAll string
	stderrAll := captureStderr(t, func() {
		outAll = captureStdout(t, func() {
			if err := runSearch([]string{"--local", "--limit", "0", "--files-with-matches", "ping"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	allLines := strings.Split(strings.TrimSpace(outAll), "\n")
	if len(allLines) != DefaultResultLimit+5 {
		t.Fatalf("--limit 0 expected %d paths, got %d", DefaultResultLimit+5, len(allLines))
	}
	if strings.Contains(stderrAll, "default --limit") {
		t.Fatalf("--limit 0 should not print the hint, got %q", stderrAll)
	}
}
