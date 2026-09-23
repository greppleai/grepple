package search

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchUsesConfiguredRemoteUnlessLocalOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "local.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, root, "init")
	runGitForTest(t, root, "remote", "add", "origin", "https://github.com/owner/current.git")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	requests := 0
	var remoteRequest api.SearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if err := json.NewDecoder(request.Body).Decode(&remoteRequest); err != nil {
			t.Error(err)
		}
		if request.URL.Path != "/public/search" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(api.SearchResponse{Results: []api.FileResult{{
			Repo: "owner/repo",
			Path: "remote.txt",
			Matches: []api.ResultMatch{{
				Line: 1,
				Text: "needle",
			}},
		}}})
	}))
	defer server.Close()

	if err := runTestSearch([]string{"--server", server.URL, "--line-only", "needle"}); err != nil {
		t.Fatal(err)
	}
	params, err := search.ResolveRequest(remoteRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.ExcludeRepo) != 1 || params.ExcludeRepo[0] != "owner/current" {
		t.Fatalf("excludeRepo=%v", params.ExcludeRepo)
	}
	if requests != 1 {
		t.Fatalf("remote requests=%d, want 1", requests)
	}
	if err := runTestSearch([]string{"--server", server.URL, "--local", "--line-only", "needle"}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("--local contacted remote; requests=%d", requests)
	}
}

// TestRemoteRequiresOptIn verifies the local-first default: a configured
// GREPPLE_SERVER is only contacted when the user passes --remote (or --server).
func TestRemoteRequiresOptIn(t *testing.T) {
	dir := chdirTemp(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	if err := os.WriteFile(filepath.Join(dir, "local.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(api.SearchResponse{})
	}))
	defer server.Close()
	t.Setenv("GREPPLE_SERVER", server.URL)

	// Default is local-only: the configured GREPPLE_SERVER must NOT be contacted.
	_ = captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "needle"}); err != nil {
			t.Fatal(err)
		}
	})
	if requests != 0 {
		t.Fatalf("remote contacted without opt-in; requests=%d", requests)
	}

	// --remote opts in and uses the configured server.
	_ = captureStdout(t, func() {
		if err := runTestSearch([]string{"--remote", "--line-only", "needle"}); err != nil {
			t.Fatal(err)
		}
	})
	if requests != 1 {
		t.Fatalf("--remote should contact the configured server once; requests=%d", requests)
	}
}
