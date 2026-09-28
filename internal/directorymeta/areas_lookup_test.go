package directorymeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAreaIndexGroupsMixedPathsWithoutDuplicatingMembership(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"first", "second"} {
		path := filepath.Join(root, directory, "source.go")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package example\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		files, err := FilesForDirectory(root, filepath.Dir(path), []string{directory + "/source.go"})
		if err != nil {
			t.Fatal(err)
		}
		files[0].Kind, files[0].Description, files[0].Areas = "production", "Example source", []string{"lookup"}
		if err := Write(root, filepath.Dir(path), Metadata{Description: "Example directory", Responsibilities: []string{"Example"}, Files: files}); err != nil {
			t.Fatal(err)
		}
	}
	refs, err := AreaIndex(root, []string{"first/source.go", "first/source.go", filepath.Join(root, "second", "source.go"), "grepple.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0].Path != "first/source.go" || refs[1].Path != "second/source.go" || refs[0].Status != StatusCurrent || refs[1].Status != StatusCurrent {
		t.Fatalf("mixed path references = %+v", refs)
	}
}
