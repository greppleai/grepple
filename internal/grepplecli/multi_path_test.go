package grepplecli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMultiPathFixture(t *testing.T) string {
	t.Helper()
	dir := chdirTemp(t)
	for _, path := range []string{"src/a.txt", "scripts/b.txt", "other/c.txt"} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("needle\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFilesListsMultipleDirectoryRoots(t *testing.T) {
	writeMultiPathFixture(t)
	out := captureStdout(t, func() {
		if err := runSearch([]string{"--files", "--limit", "0", "src", "scripts"}); err != nil {
			t.Fatal(err)
		}
	})
	if want := "scripts/b.txt\nsrc/a.txt\n"; out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestContentSearchAcceptsMultipleExplicitPaths(t *testing.T) {
	dir := writeMultiPathFixture(t)
	out := captureStdout(t, func() {
		if err := runSearch([]string{
			"--line-only", "needle",
			filepath.Join(dir, "src", "a.txt"),
			filepath.Join(dir, "scripts"),
		}); err != nil {
			t.Fatal(err)
		}
	})
	if want := "scripts/b.txt:1:needle\nsrc/a.txt:1:needle\n"; out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}
