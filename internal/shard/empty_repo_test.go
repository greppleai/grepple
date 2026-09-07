package shard

import (
	"os"
	"os/exec"
	"testing"

	"grepple/internal/repository"
)

// TestIndexRepoSkipsEmptyRepository verifies an empty repository (no commits)
// is indexed gracefully: indexRepo must not error, must record empty coverage,
// and must not invoke zoekt-git-index (which fails without a HEAD and can leave
// a partial shard). Regression for a crash when indexing a commit-less repo.
func TestIndexRepoSkipsEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	src := tmp + "/src"
	if out, err := exec.Command("git", "init", "-q", "-b", "main", src).CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	root := tmp + "/repos"
	reg := repository.NewRegistry(root)
	info, err := reg.Add("owner/empty", "file://"+src, "", "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if info.Head != nil {
		t.Fatalf("empty repo should have nil Head, got %v", *info.Head)
	}

	indexDir := tmp + "/idx"
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &shardImpl{
		registry: reg,
		root:     root,
		zoektService: &zoektServiceImpl{
			options:   zoektOptions{indexDir: indexDir, repoRoot: root},
			coverage:  map[string]zoektCoverage{},
			directory: loadDirectory(tmp + "/directory.json"),
		},
	}
	// Must not error or panic even though zoekt-git-index would fail here.
	if err := s.indexRepo(info); err != nil {
		t.Fatalf("indexRepo on empty repo should succeed, got: %v", err)
	}
	cov, ok := s.zoektService.coverageFor(info.Repo)
	if !ok || !cov.known || cov.indexed {
		t.Fatalf("expected known, un-indexed coverage for empty repo, got %+v (ok=%v)", cov, ok)
	}
	entries, _ := os.ReadDir(indexDir)
	if len(entries) != 0 {
		t.Fatalf("no shard should be created for an empty repo, found %d entries", len(entries))
	}
}

// TestIndexExistingReposSkipsEmptyRepository covers the startup path: a
// commit-less repo already on disk must not fail startup (which would crash-loop
// the shard container).
func TestReconcileRepoIndexSkipsEmptyRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	root := tmp + "/repos"
	// Create an empty repo directly under the registry root and re-init so it is
	// discovered on disk (mirrors a shard restart).
	empty := root + "/owner/empty"
	if out, err := exec.Command("git", "init", "-q", "-b", "main", empty).CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	reg := repository.NewRegistry(root)
	if err := reg.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(reg.List()) != 1 {
		t.Fatalf("expected 1 discovered repo, got %d", len(reg.List()))
	}
	indexDir := tmp + "/idx"
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &shardImpl{
		registry: reg,
		root:     root,
		zoektService: &zoektServiceImpl{
			options:   zoektOptions{indexDir: indexDir, repoRoot: root},
			coverage:  map[string]zoektCoverage{},
			directory: loadDirectory(tmp + "/directory.json"),
		},
	}
	for _, repo := range reg.List() {
		if err := s.reconcileRepoIndex(repo); err != nil {
			t.Fatalf("startup indexing must not fail on an empty repo, got: %v", err)
		}
	}
	if entries, _ := os.ReadDir(indexDir); len(entries) != 0 {
		t.Fatalf("no shard should be created for an empty repo, found %d entries", len(entries))
	}
}
