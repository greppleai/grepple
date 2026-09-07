package shard

import (
	"grepple/internal/api"
	"os"
	"path/filepath"
	"testing"
)

func strptr(s string) *string { return &s }

// TestUpToDateDecision verifies the incremental-reindex predicate that lets
// startup skip re-indexing unchanged repositories: a repo is up to date only
// when it has a commit, its head matches directory.json, and its shard exists.
func TestUpToDateDecision(t *testing.T) {
	indexDir := t.TempDir()
	dir := loadDirectory(filepath.Join(t.TempDir(), "directory.json"))
	service := &zoektServiceImpl{
		options:   zoektOptions{indexDir: indexDir},
		directory: dir,
		coverage:  map[string]zoektCoverage{},
	}
	repo := "owner/repo"
	info := api.RepoInfo{Repo: repo, Head: strptr("sha-1"), Dir: t.TempDir()}

	// No directory entry yet -> must (re)index.
	if service.upToDate(info) {
		t.Fatal("no directory entry should not be up to date")
	}
	// Directory matches but the shard file is missing -> must reindex.
	dir.set(repo, "sha-1")
	if service.upToDate(info) {
		t.Fatal("missing shard should not be up to date")
	}
	// Create the shard file for this repo.
	shard := filepath.Join(indexDir, zoektShardPrefix(repo)+"_v16.00000.zoekt")
	if err := os.WriteFile(shard, []byte("shard"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Head matches + shard exists -> up to date (skip reindex).
	if !service.upToDate(info) {
		t.Fatal("matching head with shard present should be up to date")
	}
	// Head changed -> must reindex even though a shard exists.
	changed := api.RepoInfo{Repo: repo, Head: strptr("sha-2"), Dir: info.Dir}
	if service.upToDate(changed) {
		t.Fatal("changed head should not be up to date")
	}
	// Empty repo (nil head) -> never up to date.
	empty := api.RepoInfo{Repo: repo, Head: nil, Dir: info.Dir}
	if service.upToDate(empty) {
		t.Fatal("empty repo should not be up to date")
	}
}
