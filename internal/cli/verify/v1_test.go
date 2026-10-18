package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestVerifyV1RejectsOldSchemaAndReportsInvalid(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}})
	path := directorymeta.RepositoryPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Build(application, nil)
	if err != nil || len(report.Invalid) != 1 || report.Invalid[0] != "." || report.Schema != "grepple-metadata-verification-v1" || len(report.Issues["."]) == 0 || !strings.Contains(report.Issues["."][0], "invalid") {
		t.Fatalf("old schema: %+v: %v", report, err)
	}
}

func TestVerifyV1HonorsSelectedSourceFiles(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "selected.go")
	ignored := filepath.Join(root, "ignored.go")
	for _, name := range []string{selected, ignored} {
		if err := os.WriteFile(name, []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := filedigest.SHA256Hex(selected)
	if err != nil {
		t.Fatal(err)
	}
	entry := directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{Name: "selected.go", Checksum: digest, Description: "Selected source.", Purpose: "Testing", DomainAreas: []string{"API"}}}}
	if err := directorymeta.WriteDirectoryV1(root, root, entry); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var output bytes.Buffer
	var exitCode int
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}, Exit: func(code int) { exitCode = code }})
	report, err := Build(application, []string{"selected.go"})
	if err != nil || report.Valid != 1 || report.Directories != 1 || len(report.Stale) != 0 {
		t.Fatalf("scoped report: %+v: %v", report, err)
	}
	if err := Execute(application, &Args{Paths: []string{"selected.go"}, JSON: true}); err != nil || exitCode != 0 || !strings.Contains(output.String(), `"schema": "grepple-metadata-verification-v1"`) {
		t.Fatalf("JSON output %q, exit %d: %v", output.String(), exitCode, err)
	}
	if err := os.WriteFile(selected, []byte("package demo\n// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Execute(application, &Args{Paths: []string{"selected.go"}}); err != nil || exitCode != 1 || !strings.Contains(output.String(), "S .") || !strings.Contains(output.String(), "stale checksum for \"selected.go\"") {
		t.Fatalf("stale output %q, exit %d: %v", output.String(), exitCode, err)
	}
}

func TestVerifyV1MissingEntryFailsThenCurrentEntryPasses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var output bytes.Buffer
	exitCode := 0
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}, Exit: func(code int) { exitCode = code }})
	if err := Execute(application, &Args{}); err != nil || exitCode != 1 || !strings.Contains(output.String(), "M .") {
		t.Fatalf("missing report=%q exit=%d err=%v", output.String(), exitCode, err)
	}
	digest, err := filedigest.SHA256Hex(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{Name: "main.go", Checksum: digest, Description: "Main source.", Purpose: "Implementation"}}}); err != nil {
		t.Fatal(err)
	}
	report, err := Build(application, nil)
	if err != nil || report.Valid != 1 || report.Directories != 1 || len(report.Missing) != 0 || len(report.Stale) != 0 || len(report.Invalid) != 0 {
		t.Fatalf("current report=%+v err=%v", report, err)
	}
}
