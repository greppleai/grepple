package repository

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// makeSourceRepo creates a local git repository whose default branch is
// defaultBranch (deliberately not "main") and returns a file:// clone URL.
func makeSourceRepo(t *testing.T, defaultBranch string) string {
	t.Helper()
	src := t.TempDir()
	gitRun(t, src, "init", "-q", "-b", defaultBranch)
	gitRun(t, src, "config", "user.email", "test@example.com")
	gitRun(t, src, "config", "user.name", "Test")
	if err := exec.Command("bash", "-c", "echo hello > "+filepath.Join(src, "file.txt")).Run(); err != nil {
		t.Fatal(err)
	}
	gitRun(t, src, "add", ".")
	gitRun(t, src, "commit", "-q", "-m", "initial")
	return "file://" + src
}

// TestAddClonesRemoteDefaultBranchIgnoringWrongRef verifies the fix: cloning
// no longer forces the caller's ref (e.g. "main"). A repo whose default branch
// is "develop" must clone successfully even when a wrong ref is supplied, and
// the recorded ref must be the real default branch.
func TestAddClonesRemoteDefaultBranchIgnoringWrongRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	url := makeSourceRepo(t, "develop")
	reg := NewRegistry(t.TempDir())

	// Pass a deliberately wrong ref; before the fix this became
	// `git clone --branch main` and failed because no "main" branch exists.
	info, err := reg.Add("owner/repo", url, "main", "")
	if err != nil {
		t.Fatalf("Add should succeed by cloning the remote default branch: %v", err)
	}
	if info.Ref != "develop" {
		t.Fatalf("recorded ref = %q, want the remote default branch %q", info.Ref, "develop")
	}
	if _, err := exec.Command("git", "-C", info.Dir, "rev-parse", "--verify", "develop").Output(); err != nil {
		t.Fatalf("cloned working copy is not on develop: %v", err)
	}
}

// TestAddClonesDefaultWhenNoRef covers the pre-existing empty-ref path still
// resolving to the remote default branch.
func TestAddClonesDefaultWhenNoRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	url := makeSourceRepo(t, "trunk")
	reg := NewRegistry(t.TempDir())
	info, err := reg.Add("owner/repo", url, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Ref != "trunk" {
		t.Fatalf("recorded ref = %q, want %q", info.Ref, "trunk")
	}
}
