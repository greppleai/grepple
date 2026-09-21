package gitcontext

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitRepoIDFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	runGitForTest(t, root, "init")
	runGitForTest(t, root, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	nested := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if got := From(nested); got != "acme/widgets" {
		t.Fatalf("repo=%q", got)
	}
}

func TestGitRepoIDUsesNonOriginRemote(t *testing.T) {
	root := t.TempDir()
	runGitForTest(t, root, "init")
	runGitForTest(t, root, "remote", "add", "upstream", "https://github.com/acme/upstream.git")
	if got := From(root); got != "acme/upstream" {
		t.Fatalf("repo=%q", got)
	}
}

func TestGitRepoIDRecognizesWorktreeGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /tmp/not-used\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findGitRoot(filepath.Join(root, "child")); got != root {
		t.Fatalf("root=%q want %q", got, root)
	}
}

func TestRepoIDFromRemote(t *testing.T) {
	tests := map[string]string{
		"git@github.com:owner/repo.git":        "owner/repo",
		"https://github.com/owner/repo.git":    "owner/repo",
		"ssh://git@github.com/owner/repo.git":  "owner/repo",
		"https://gitlab.test/a/owner/repo.git": "owner/repo",
		"":                                     "",
		"https://github.com/only-one":          "",
	}
	for remote, want := range tests {
		if got := repoIDFromRemote(remote); got != want {
			t.Errorf("repoIDFromRemote(%q)=%q want %q", remote, got, want)
		}
	}
}

func TestGitRepoIDOutsideRepository(t *testing.T) {
	if got := From(t.TempDir()); got != "" {
		t.Fatalf("repo=%q", got)
	}
}

func runGitForTest(t *testing.T, directory string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", directory}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
