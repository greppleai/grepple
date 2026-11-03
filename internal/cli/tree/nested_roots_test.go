package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNestedTreeRootsCachesAncestorsAndMisses(t *testing.T) {
	working := t.TempDir()
	for _, marker := range []string{".git", "checkout/.grepple", "checkout/deeper/.git"} {
		path := filepath.Join(working, marker)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		// A worktree .git file is a boundary too.
		if err := os.WriteFile(path, []byte("marker"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	roots := nestedTreeRoots{working: working, cache: map[string]string{working: ""}}
	for _, tc := range []struct{ directory, root string }{
		{"ordinary/a/b", ""},
		{"ordinary/a/c", ""},
		{"checkout/pkg/sub", "checkout"},
		{"checkout/pkg/other", "checkout"},
		{"checkout/deeper/pkg", "checkout/deeper"},
	} {
		directory := filepath.Join(working, tc.directory)
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		want := ""
		if tc.root != "" {
			want = filepath.Join(working, tc.root)
		}
		if got := roots.find(directory); got != want {
			t.Fatalf("find(%s) = %q, want %q", tc.directory, got, want)
		}
		// All intermediate ancestors up to the nearest boundary are cached,
		// including negative lookups. Repeated files require no marker probes.
		for path := directory; path != working; path = filepath.Dir(path) {
			if got, ok := roots.cache[path]; !ok || got != want {
				t.Fatalf("cache[%s] = %q, %t; want %q", path, got, ok, want)
			}
			if path == want {
				break
			}
		}
	}
	// A new invocation must see boundaries created after a negative lookup.
	if err := os.Mkdir(filepath.Join(working, "ordinary", ".grepple"), 0700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(working, "ordinary", "a")
	if got := roots.find(directory); got != "" {
		t.Fatalf("negative lookup was not cached: %q", got)
	}
	fresh := nestedTreeRoots{working: working, cache: map[string]string{working: ""}}
	if got := fresh.find(directory); got != filepath.Join(working, "ordinary") {
		t.Fatalf("fresh invocation missed new boundary: %q", got)
	}
}
