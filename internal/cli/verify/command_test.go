package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
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
		entry := directorymeta.DirectoryV1{Files: []directorymeta.FileV1{}}
		if directory == filepath.Join(root, "pkg") {
			checksum, hashErr := filedigest.SHA256Hex(filepath.Join(directory, "file.go"))
			if hashErr != nil {
				t.Fatal(hashErr)
			}
			entry.Files = append(entry.Files, directorymeta.FileV1{Name: "file.go", Checksum: checksum, Description: "Contains package code.", Purpose: "Implementation"})
		}
		if err := directorymeta.WriteDirectoryV1(root, directory, entry); err != nil {
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
	if err != nil || len(report.Stale) != 1 || report.Stale[0] != "pkg" || len(report.Issues["pkg"]) == 0 {
		t.Fatalf("stale report=%+v err=%v", report, err)
	}
}
