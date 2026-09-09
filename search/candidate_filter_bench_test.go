package search

import (
	"os"
	"path/filepath"
	"testing"
)

// buildIgnoreTree creates a nested directory tree with a .gitignore at every
// level and returns the root plus the list of leaf files (as absolute paths),
// simulating the candidate list Zoekt hands to SearchFiles.
func buildIgnoreTree(tb testing.TB, depth, filesPerDir int) (string, []string) {
	tb.Helper()
	root := tb.TempDir()
	var files []string
	dir := root
	for level := 0; level < depth; level++ {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nnode_modules/\n# comment\n"), 0o644); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < filesPerDir; f++ {
			p := filepath.Join(dir, "file.go")
			if f > 0 {
				p = filepath.Join(dir, "file"+string(rune('a'+f))+".go")
			}
			if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
			files = append(files, p)
		}
		dir = filepath.Join(dir, "sub")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
	}
	return root, files
}

// BenchmarkCandidateFilterWithIgnore measures the removed behavior: the per-file
// pathIgnoredFromRoot lookup that re-reads parent .gitignore files.
func BenchmarkCandidateFilterWithIgnore(b *testing.B) {
	root, files := buildIgnoreTree(b, 12, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, file := range files {
			dp := file
			_ = withinRoot(file, root) && pathMatchesGlobs(dp, nil) && !pathContainsGitDirectory(file) && !pathIgnoredFromRoot(file, root)
		}
	}
}

// BenchmarkCandidateFilterNoIgnore measures the current behavior: cheap
// in-memory guards only (Zoekt candidates are already git-tracked).
func BenchmarkCandidateFilterNoIgnore(b *testing.B) {
	root, files := buildIgnoreTree(b, 12, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, file := range files {
			dp := file
			_ = withinRoot(file, root) && pathMatchesGlobs(dp, nil) && !pathContainsGitDirectory(file)
		}
	}
}
