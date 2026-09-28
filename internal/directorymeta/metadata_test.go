package directorymeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataRoundTripAndTreeSummary(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "parser")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, root, Metadata{Description: "Repository root.", Responsibilities: []string{"Coordinate packages."}}); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, child, Metadata{Description: "Parses source.", Responsibilities: []string{"Parse files."}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := Read(root, child)
	if err != nil || loaded.Description != "Parses source." {
		t.Fatalf("metadata=%+v err=%v", loaded, err)
	}
	summary := TreeSummary(root, []string{filepath.Join(child, "source.go")}, 1)
	if !strings.Contains(summary, ". — Repository root.") || !strings.Contains(summary, "parser/ — Parses source.") {
		t.Fatalf("summary=%q", summary)
	}
}

func TestDirectoriesAndFileChecksumsAreDeterministic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "file.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package nested\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directories, err := Directories(root, []string{"nested/file.go"})
	if err != nil || len(directories) != 2 || directories[0] != root {
		t.Fatalf("directories=%v err=%v", directories, err)
	}
	files, err := FilesForDirectory(root, filepath.Dir(path), []string{"nested/file.go"})
	if err != nil || len(files) != 1 || files[0].Path != "file.go" || len(files[0].Checksum) != 64 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
}
