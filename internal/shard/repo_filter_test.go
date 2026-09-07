package shard

import (
	"os"
	"path/filepath"
	"testing"

	"grepple/internal/api"
	"grepple/internal/repository"
)

// TestRunSearchRequestSkipsShardWithoutMatchingRepo verifies the repo pre-filter:
// when --repo matches none of a shard's repositories the shard returns an empty
// result immediately, and when it does match, results still come back.
func TestRunSearchRequestSkipsShardWithoutMatchingRepo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "owner", "foo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package foo\n// needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	registry.Record("owner/foo", "", "HEAD", dir)
	state := &shardImpl{registry: registry, root: root}

	// The scan fallback resolves candidate files relative to the working dir.
	t.Chdir(root)
	query := "needle"

	// --repo matches nothing on this shard: fast path returns empty, no scan.
	miss, err := state.runSearchRequest(api.SearchRequest{Query: &query, Repo: []string{"owner/bar"}}, "")
	if err != nil {
		t.Fatalf("miss search: %v", err)
	}
	if len(miss.Results) != 0 {
		t.Fatalf("expected 0 results for non-matching --repo, got %d", len(miss.Results))
	}

	// --repo matches: results still come back.
	hit, err := state.runSearchRequest(api.SearchRequest{Query: &query, Repo: []string{"owner/foo"}}, "")
	if err != nil {
		t.Fatalf("hit search: %v", err)
	}
	if len(hit.Results) != 1 || hit.Results[0].Repo != "owner/foo" {
		t.Fatalf("expected 1 result in owner/foo, got %#v", hit.Results)
	}
}

// TestFilesModeScopesWalkToMatchingRepos verifies -l/--files with --repo lists
// only the matching repository's files (and skips other repos entirely).
func TestFilesModeScopesWalkToMatchingRepos(t *testing.T) {
	root := t.TempDir()
	for _, repo := range []string{"owner/foo", "owner/bar"} {
		dir := filepath.Join(root, filepath.FromSlash(repo))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	registry.Record("owner/foo", "", "HEAD", filepath.Join(root, "owner", "foo"))
	registry.Record("owner/bar", "", "HEAD", filepath.Join(root, "owner", "bar"))
	state := &shardImpl{registry: registry, root: root}
	t.Chdir(root)

	resp, err := state.runSearchRequest(api.SearchRequest{Files: true, Globs: []string{"**/*.tf"}, Repo: []string{"owner/foo"}}, "")
	if err != nil {
		t.Fatalf("files search: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Path != "owner/foo/main.tf" {
		t.Fatalf("expected only owner/foo/main.tf, got %#v", resp.Results)
	}
}
