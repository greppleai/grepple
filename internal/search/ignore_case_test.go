package search

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSearchFilesIgnoreCase guards the case-insensitive substring path, which
// now folds the whole file once instead of per line. Matching (and match line
// numbers) must be identical to a per-line fold.
func TestSearchFilesIgnoreCase(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "owner/repo/file.go")
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	// needle on lines 2 and 4 with varied casing; line 3 has no match.
	body := "package x\nvar HelloWorld = 1\nvar other = 2\nfunc helloWORLD() {}\n"
	if err := os.WriteFile(abs, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	insensitive, err := Files(Params{Query: "helloworld", IgnoreCase: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(insensitive) != 1 {
		t.Fatalf("ignore-case: expected 1 file, got %d", len(insensitive))
	}
	got := insensitive[0].MatchLines
	if len(got) != 2 || !got[2] || !got[4] {
		t.Fatalf("ignore-case match lines = %v, want {2,4}", got)
	}

	sensitive, err := Files(Params{Query: "helloworld"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sensitive) != 0 {
		t.Fatalf("case-sensitive: expected no matches for %q, got %d", "helloworld", len(sensitive))
	}
}
