package search

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGitignoreCacheMatchesUncached ensures the cached ignore check returns the
// same verdict as the per-file path for both ignored and non-ignored files.
func TestGitignoreCacheMatchesUncached(t *testing.T) {
	root := t.TempDir()
	// Nested tree with .gitignore rules at two levels.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\nbuild/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(filepath.Join(root, "build"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("secret.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path        string
		wantIgnored bool
	}{
		{filepath.Join(root, "main.go"), false},
		{filepath.Join(root, "debug.log"), true}, // *.log
		{filepath.Join(sub, "app.go"), false},
		{filepath.Join(sub, "secret.txt"), true},     // pkg/.gitignore
		{filepath.Join(root, "build", "y.go"), true}, // build/ (top level)
	}
	cache := gitignoreCache{}
	for _, c := range cases {
		// The cached check must produce the correct verdict, and match the
		// per-file (fresh-cache) path.
		if got := pathIgnoredFromRootCached(c.path, root, cache); got != c.wantIgnored {
			t.Errorf("%s: cached ignored=%v, want %v", c.path, got, c.wantIgnored)
		}
		if got := pathIgnoredFromRoot(c.path, root); got != c.wantIgnored {
			t.Errorf("%s: uncached ignored=%v, want %v", c.path, got, c.wantIgnored)
		}
	}
}

func BenchmarkPathIgnoredCachedBatch(b *testing.B) {
	root, files := buildIgnoreTree(b, 12, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := gitignoreCache{}
		for _, f := range files {
			_ = pathIgnoredFromRootCached(f, root, cache)
		}
	}
}
