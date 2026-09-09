package search

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSearchFilesSkipsBinary guards the NUL-byte (binary) detection, which was
// changed from strings.IndexByte(string(bytes)) to a zero-copy
// bytes.IndexByte(bytes). A file containing a NUL must be skipped; a plain text
// file with the same needle must still match.
func TestSearchFilesSkipsBinary(t *testing.T) {
	root := t.TempDir()
	write := func(path string, data []byte) {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("owner/repo/text.go", []byte("package sample\n// needle here\n"))
	write("owner/repo/blob.bin", []byte("needle\x00needle\n"))

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	matches, err := Files(Params{Query: "needle", MaxSegments: DefaultMaxSegments}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].DisplayPath != "owner/repo/text.go" {
		t.Fatalf("expected only the text file to match, got %#v", matches)
	}
}
