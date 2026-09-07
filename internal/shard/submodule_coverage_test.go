package shard

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInspectZoektCoverageSkipsSubmoduleDirs reproduces a submodule coverage
// failure: a git superrepo with a submodule gitlink lists that submodule as a tracked
// path that is a directory on disk. Coverage inspection must skip it (submodule
// contents aren't indexed from the superrepo) and keep the repo covered, instead
// of failing on the "is a directory" read error and marking the whole repo
// uncovered — which previously dragged the entire shard into a scan fallback.
func TestInspectZoektCoverageSkipsSubmoduleDirs(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	git("init", "-q")
	// A normal indexable file.
	if err := os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A tracked path that we then turn into a directory on disk, mimicking a
	// submodule gitlink (git keeps it in the index; on disk it is a directory).
	if err := os.WriteFile(filepath.Join(dir, "repos"), []byte("gitlink placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "keep.go", "repos")
	git("commit", "-q", "-m", "init")

	if err := os.Remove(filepath.Join(dir, "repos")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "repos", "ep_dbt_core"), 0o755); err != nil {
		t.Fatal(err)
	}

	cov := inspectZoektCoverage(dir)
	if !cov.known {
		t.Fatal("coverage must stay known when a tracked path is a directory (submodule gitlink)")
	}
	if !cov.indexed {
		t.Fatal("the normal file should still count as indexed")
	}
}
