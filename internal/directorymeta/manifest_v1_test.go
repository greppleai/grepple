package directorymeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesForDirectoryV1HonorsSelectionAndChecksums(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"one.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FilesForDirectoryV1(root, root, []string{"one.go", "one.go", "other.go", ".grepple/grepple.yaml"})
	if err != nil || len(got) != 2 || got[0].Name != "one.go" || got[1].Name != "other.go" || len(got[0].Checksum) != 64 {
		t.Fatalf("manifest: %+v: %v", got, err)
	}
	if err := os.Remove(filepath.Join(root, "one.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := FilesForDirectoryV1(root, root, []string{"one.go"}); err == nil {
		t.Fatal("missing selected source accepted")
	}
}
