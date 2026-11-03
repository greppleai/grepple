package directorymeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUninitializedCheckoutAncestorBoundaries(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "checkout")
	child := filepath.Join(root, "pkg", "deep")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	// Worktrees use a .git file; ordinary checkouts use a directory.
	for _, directoryMarker := range []bool{false, true} {
		marker := filepath.Join(root, ".git")
		if directoryMarker {
			if err := os.Mkdir(marker, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(marker, []byte("gitdir: elsewhere\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, start := range []string{root, child} {
			if got, ok := UninitializedCheckout(start); !ok || got != root {
				t.Fatalf("UninitializedCheckout(%s) = %q, %t", start, got, ok)
			}
		}
		if _, ok := UninitializedCheckout(parent); ok {
			t.Fatal("parent of a checkout must retain nested discovery")
		}
		if err := os.Remove(marker); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".grepple"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, ok := UninitializedCheckout(child); ok {
		t.Fatal("existing checkout metadata must retain full inspection")
	}
	if err := os.Remove(filepath.Join(root, ".grepple")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", filepath.Join(root, ".grepple")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, ok := UninitializedCheckout(child); ok {
		t.Fatal("broken metadata symlink must not be treated as absent")
	}
}
