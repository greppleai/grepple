package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/memory"
	search "github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func memorySearchFixture(t *testing.T) string {
	t.Helper()
	root := chdirTemp(t)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	writeGraphSource(t, root, "a.go", "package a\nfunc Run() { // needle\n}\n")
	writeGraphSource(t, root, "b.go", "package a\nfunc Other() { // needle\n}\n")
	return root
}
func storeSearchMemory(t *testing.T, root, name, path string, lines []int) {
	t.Helper()
	_, digest, err := memory.Source(root, path)
	if err != nil {
		t.Fatal(err)
	}
	h := memory.Header{Schema: "grepple-memory-v1", Name: name, Description: "A useful hint", Files: []memory.Attachment{{Path: path, Checksum: digest, LineRange: lines}}}
	if err := memory.Save(root, h, []byte("This body is private and must never be in search output"), false); err != nil {
		t.Fatal(err)
	}
}
func searchCapture(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() {
		if err := Run(append([]string{"search"}, args...)); err != nil {
			t.Fatal(err)
		}
	})
}
func TestSearchMemoryWindowAndZeroMemoryParity(t *testing.T) {
	root := memorySearchFixture(t)
	baseline := searchCapture(t, "--line-only", "-F", "needle", ".")
	if strings.Contains(baseline, "Memories located:") {
		t.Fatal("absent directory produced footer")
	}
	storeSearchMemory(t, root, "first", "a.go", []int{2, 2})
	storeSearchMemory(t, root, "second", "b.go", []int{2, 2})
	output := searchCapture(t, "--line-only", "-F", "needle", ".")
	if !strings.Contains(output, "Memories located:") || !strings.Contains(output, "grepple memory recall first") || !strings.Contains(output, "grepple memory recall second") || strings.Contains(output, "This body is private") {
		t.Fatalf("unexpected footer %q", output)
	}
	if one := searchCapture(t, "--line-only", "--limit", "1", "-F", "needle", "."); !strings.Contains(one, "recall first") || strings.Contains(one, "recall second") {
		t.Fatalf("limit crossed page: %q", one)
	}
	if two := searchCapture(t, "--line-only", "--skip", "1", "--limit", "1", "-F", "needle", "."); strings.Contains(two, "recall first") || !strings.Contains(two, "recall second") {
		t.Fatalf("skip crossed page: %q", two)
	}
	// A non-qualifying memory may not change a byte of the ordinary output.
	if err := memory.Delete(root, "first"); err != nil {
		t.Fatal(err)
	}
	if err := memory.Delete(root, "second"); err != nil {
		t.Fatal(err)
	}
	if got := searchCapture(t, "--line-only", "-F", "needle", "."); got != baseline {
		t.Fatalf("no-memory output changed\nwant %q\ngot  %q", baseline, got)
	}
}
func TestSearchMemoryJSONFormsAndSuppression(t *testing.T) {
	root := memorySearchFixture(t)
	storeSearchMemory(t, root, "first", "a.go", []int{2, 2})
	for _, flag := range []string{"--json", "--json-matches"} {
		output := searchCapture(t, flag, "-F", "needle", "a.go")
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal([]byte(output), &decoded); err != nil {
			t.Fatalf("invalid %s JSON: %v", flag, err)
		}
		var refs []wire.MemoryReference
		var status wire.MemoryStatus
		if err := json.Unmarshal(decoded["memories"], &refs); err != nil || len(refs) != 1 || refs[0].Name != "first" {
			t.Fatalf("%s refs %q %v", flag, decoded["memories"], err)
		}
		if err := json.Unmarshal(decoded["memoryStatus"], &status); err != nil || !status.Complete || status.Omitted != 0 {
			t.Fatalf("%s status %+v %v", flag, status, err)
		}
		if strings.Contains(output, "This body is private") || strings.Contains(output, "checksum") || strings.Contains(output, "lineRange") {
			t.Fatalf("private details in JSON %q", output)
		}
	}
	var page wire.SearchResponse
	if err := json.Unmarshal([]byte(searchCapture(t, "--json", "--limit", "1", "-F", "needle", ".")), &page); err != nil {
		t.Fatal(err)
	}
	if page.Metadata == nil || page.Metadata.Page.Returned != 1 || page.Metadata.Page.Complete || page.Metadata.NextCommand == "" || len(page.Memories) != 1 {
		t.Fatalf("memory changed page metadata: %+v", page.Metadata)
	}
	for _, args := range [][]string{
		{"--files", "*.go"}, {"--files-with-matches", "-F", "needle", "a.go"}, {"--count", "-F", "needle", "a.go"},
		{"--count-summary", "-F", "needle", "a.go"}, {"--outline", "a.go"}, {"--at", "a.go:2"},
		{"--related", "-F", "needle", "a.go"},
	} {
		output := searchCapture(t, args...)
		if strings.Contains(output, "Memories located:") || strings.Contains(output, "recall first") {
			t.Fatalf("suppressed mode %v: %q", args, output)
		}
	}
}
func TestSearchMemoryInvalidEntryAndNoStructuralWidening(t *testing.T) {
	root := memorySearchFixture(t)
	// Structural StartLine/EndLine may span the enclosing function even when
	// only the comment on a different line matched. Use matching Line only.
	writeGraphSource(t, root, "a.go", "package a\nfunc Run() {\n // needle\n}\n")
	storeSearchMemory(t, root, "range", "a.go", []int{2, 2})
	if result := searchCapture(t, "--line-only", "-F", "needle", "a.go"); strings.Contains(result, "recall range") {
		t.Fatalf("enclosing scope widened match: %q", result)
	}
	dir := filepath.Join(root, ".grepple", "memories")
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("---\ninvalid: true\n---\nbody"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := searchCapture(t, "--line-only", "-F", "needle", "a.go"); !strings.Contains(result, "memory references incomplete") || strings.Contains(result, "recall range") {
		t.Fatalf("missing incomplete marker: %q", result)
	}
}

func TestSearchMemoryScopeGatesAndNestedWorkingDirectory(t *testing.T) {
	root := memorySearchFixture(t)
	storeSearchMemory(t, root, "first", "a.go", []int{2, 2})
	local := []wire.FileResult{{Path: "a.go", Matches: []wire.ResultMatch{{Line: 2, StartLine: 1, EndLine: 3}}}}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "", false, local); found == nil || len(found.References) != 1 {
		t.Fatalf("local evidence %+v", found)
	}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "", true, local); found != nil {
		t.Fatalf("mixed remote %+v", found)
	}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "https://example.test", false, local); found != nil {
		t.Fatalf("explicit server %+v", found)
	}
	for _, opt := range []*Options{{RemoteOnly: true}, {MixedRemote: true}, {Cursor: "token"}, {Stdin: true}, {Count: true}, {CountByRepo: true}, {CountSummary: true}, {FilesWithMatches: true}, {Outline: true}, {RelatedExplicit: true}, {Params: search.Params{At: "a.go:2"}}, {Params: search.Params{Files: true}}, {Params: search.Params{Repo: []string{"org/repo"}}}} {
		if found := selectedMemoryReferences(testCommandContext(), opt, "", false, local); found != nil {
			t.Fatalf("scope gate %+v => %+v", opt, found)
		}
	}
	remotePage := []wire.FileResult{{Path: "a.go", Repo: "org/repo", Matches: []wire.ResultMatch{{Line: 2}}}}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "", false, remotePage); found != nil {
		t.Fatalf("remote identity %+v", found)
	}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "", false, []wire.FileResult{{Path: "../outside.go", Matches: []wire.ResultMatch{{Line: 2}}}}); found != nil {
		t.Fatalf("out-of-root path %+v", found)
	}
	writeGraphSource(t, root, "nested/x.go", "package nested\nfunc X() { // needle\n}\n")
	storeSearchMemory(t, root, "nested", "nested/x.go", []int{2, 2})
	t.Chdir(filepath.Join(root, "nested"))
	nested := []wire.FileResult{{Path: "x.go", Matches: []wire.ResultMatch{{Line: 2}}}}
	if found := selectedMemoryReferences(testCommandContext(), &Options{}, "", false, nested); found == nil || len(found.References) != 1 || found.References[0].Name != "nested" {
		t.Fatalf("nested working directory %+v", found)
	}
}

func TestSearchMemoryJSONNoMemoryByteParity(t *testing.T) {
	root := memorySearchFixture(t)
	baseline := searchCapture(t, "--json-matches", "-F", "needle", "a.go")
	if strings.Contains(baseline, "memoryStatus") || strings.Contains(baseline, "\"memories\"") {
		t.Fatalf("unexpected empty memory fields %q", baseline)
	}
	storeSearchMemory(t, root, "unrelated", "b.go", []int{2, 2})
	if got := searchCapture(t, "--json-matches", "-F", "needle", "a.go"); got != baseline {
		t.Fatalf("JSON changed without relevant memory\n%s\n%s", baseline, got)
	}
}

func TestSearchMemoryMixedRemoteIsolationEndToEnd(t *testing.T) {
	root := memorySearchFixture(t)
	storeSearchMemory(t, root, "local-note", "a.go", []int{2, 2})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := wire.FileResult{Repo: "remote/other", Path: "a.go", Matches: []wire.ResultMatch{{Line: 2, Text: "needle"}}}
		switch r.URL.Path {
		case "/public/search":
			_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: []wire.FileResult{result}})
		case "/public/search/pages":
			_ = json.NewEncoder(w).Encode(apiclient.SearchPage{Complete: true, PageID: fixtureCursor("A"), Results: []wire.FileResult{result}})
		default:
			t.Errorf("unexpected remote endpoint %s", r.URL.Path)
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()
	baseline := searchCapture(t, "--line-only", "-F", "needle", "a.go")
	if !strings.Contains(baseline, "recall local-note") {
		t.Fatalf("local baseline %q", baseline)
	}
	for _, mode := range [][]string{{"--server", server.URL}, {"--remote", "--server", server.URL}, {"--remote-only", "--server", server.URL}} {
		args := append(append([]string{}, mode...), "--json", "-F", "needle", "a.go")
		output := searchCapture(t, args...)
		if strings.Contains(output, "local-note") || strings.Contains(output, "\"memories\"") || strings.Contains(output, "memoryStatus") {
			t.Fatalf("remote mode %v leaked local memory: %q", mode, output)
		}
	}
}
