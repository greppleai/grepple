package directorymeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectReportsMissingCurrentAndStaleMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(root, root, []string{path})
	if inspection.Status != StatusMissing || inspection.Files["main.go"].Status != StatusMissing {
		t.Fatalf("missing inspection=%+v", inspection)
	}
	checksum, err := checksumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(root, Metadata{
		Description:      "Application package.",
		Responsibilities: []string{"Start the application."},
		Files:            []File{{Path: "main.go", Description: "Starts the application.", Checksum: checksum}},
	}); err != nil {
		t.Fatal(err)
	}
	inspection = Inspect(root, root, []string{path})
	if inspection.Status != StatusCurrent || inspection.Files["main.go"].Status != StatusCurrent || inspection.Files["main.go"].Kind != "unknown" {
		t.Fatalf("current inspection=%+v", inspection)
	}
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection = Inspect(root, root, []string{path})
	if inspection.Status != StatusStale || inspection.Files["main.go"].Status != StatusStale {
		t.Fatalf("stale inspection=%+v", inspection)
	}
}
func TestInspectRejectsInvalidSourceKind(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checksum, err := checksumFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(root, Metadata{Description: "Application.", Responsibilities: []string{"Run."}, Files: []File{{Path: "main.go", Description: "Runs.", Kind: "application", Checksum: checksum}}}); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(root, root, []string{path})
	if inspection.Status != StatusInvalid || inspection.Files["main.go"].Kind != "invalid" {
		t.Fatalf("inspection=%+v", inspection)
	}
}

func TestInspectReportsMissingFileEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, Metadata{Description: "Application package.", Responsibilities: []string{"Start the application."}}); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(root, root, []string{path})
	if inspection.Status != StatusStale || inspection.Files["main.go"].Status != StatusMissing {
		t.Fatalf("inspection=%+v", inspection)
	}
}

func TestTreeSummaryDisclosesMissingMetadata(t *testing.T) {
	root := t.TempDir()
	if summary := TreeSummary(root, nil, 1); summary != ". [metadata: missing]" {
		t.Fatalf("summary=%q", summary)
	}
}
