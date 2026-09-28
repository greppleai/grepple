package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestBuildReportsMissingAndValidDirectoryMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "file.go"), []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	context := cliruntime.NewContext(cliruntime.ContextOptions{Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}})
	report, err := Build(context, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Missing) != 2 {
		t.Fatalf("report=%+v", report)
	}
	for _, directory := range []string{root, filepath.Join(root, "pkg")} {
		files, fileErr := directorymeta.FilesForDirectory(root, directory, []string{"pkg/file.go"})
		if fileErr != nil {
			t.Fatal(fileErr)
		}
		for index := range files {
			files[index].Description = "Contains package code."
		}
		if err := directorymeta.Write(root, directory, directorymeta.Metadata{Description: "Description.", Responsibilities: []string{"Own code."}, Files: files}); err != nil {
			t.Fatal(err)
		}
	}
	report, err = Build(context, nil)
	if err != nil || report.Valid != 2 || len(report.Missing) != 0 || len(report.Stale) != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "file.go"), []byte("package pkg\nfunc changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = Build(context, nil)
	if err != nil || len(report.Stale) != 1 || report.Stale[0] != "pkg" {
		t.Fatalf("stale report=%+v err=%v", report, err)
	}
}
