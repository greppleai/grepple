package directorymeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAreaIndexTracksCurrentStaleAndInvalidMembership(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "pkg", "source.go")
	if err := os.WriteFile(path, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := FilesForDirectory(root, filepath.Join(root, "pkg"), []string{"pkg/source.go"})
	if err != nil {
		t.Fatal(err)
	}
	files[0].Description, files[0].Kind, files[0].Areas = "Owns source.", "production", []string{"outline"}
	if err := Write(root, filepath.Join(root, "pkg"), Metadata{Description: "Package.", Responsibilities: []string{"Own source."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		refs, err := AreaIndex(root, []string{"pkg/source.go"})
		if err != nil || len(refs) != 1 || refs[0].Path != "pkg/source.go" || refs[0].Status != want {
			t.Fatalf("refs=%+v err=%v want=%s", refs, err, want)
		}
	}
	check(StatusCurrent)
	if err := os.WriteFile(path, []byte("package pkg\n// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check(StatusStale)
	digest, err := checksumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files[0].Checksum = digest
	files[0].Areas = []string{"outline", "outline"}
	if err := Write(root, filepath.Join(root, "pkg"), Metadata{Description: "Package.", Responsibilities: []string{"Own source."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	refs, err := AreaIndex(root, []string{"pkg/source.go"})
	if err != nil || len(refs) != 2 || refs[0].Status != StatusInvalid || !strings.Contains(strings.Join(refs[0].Issues, " "), "duplicate area") {
		t.Fatalf("duplicate refs=%+v err=%v", refs, err)
	}
	files[0].Areas = []string{"outline"}
	if err := Write(root, filepath.Join(root, "pkg"), Metadata{Description: "Package.", Responsibilities: []string{"Own source."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "other.go"), []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refs, err = AreaIndex(root, []string{"pkg/other.go"})
	if err != nil || len(refs) != 1 || refs[0].Status != StatusStale || refs[0].Path != "pkg/source.go" {
		t.Fatalf("deleted source membership=%+v err=%v", refs, err)
	}
	for _, area := range []string{"", "Outline", "foo/bar", "two--words", "two-"} {
		if ValidArea(area) {
			t.Fatalf("accepted invalid area %q", area)
		}
	}
}
