package directorymeta

import (
	"os"
	"path/filepath"
	"testing"
)

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
	files, err := FilesForDirectoryV1(root, filepath.Dir(path), []string{"nested/file.go"})
	if err != nil || len(files) != 1 || files[0].Name != "file.go" || len(files[0].Checksum) != 64 {
		t.Fatalf("files=%+v err=%v", files, err)
	}
}
