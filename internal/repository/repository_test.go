package repository

import (
	"grepple/internal/api"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRepoFilePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "srv", "repos", "acme", "alpha")
	got, ok := SafeRepoFilePath(root, "src/f.txt")
	if !ok || got != filepath.Join(root, "src", "f.txt") {
		t.Fatalf("normal path: %q %v", got, ok)
	}
	for _, path := range []string{"../SECRET.txt", "../../etc/passwd", "/etc/passwd", ".git/config", ""} {
		if _, ok := SafeRepoFilePath(root, path); ok {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
}

func TestWalkTree(t *testing.T) {
	directory := t.TempDir()
	for _, path := range []string{"a/c", ".git"} {
		if err := os.MkdirAll(filepath.Join(directory, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"top.txt", "a/b.txt", "a/c/d.txt", ".git/config"} {
		if err := os.WriteFile(filepath.Join(directory, path), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := WalkTree(directory, 2)
	want := []api.TreeEntry{
		{Path: "a", Dir: true},
		{Path: "a/b.txt", Dir: false},
		{Path: "a/c", Dir: true},
		{Path: "top.txt", Dir: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("entry %d: got %#v want %#v", index, got[index], want[index])
		}
	}
}
