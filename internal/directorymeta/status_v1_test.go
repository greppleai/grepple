package directorymeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/filedigest"
)

func TestInspectDirectoryV1MissingCurrentStaleAndInvalid(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "main.go")
	if err := os.WriteFile(source, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selected := []string{source}
	if got := InspectDirectoryV1(root, root, selected); got.Status != StatusMissing || got.Files["main.go"].Status != StatusMissing {
		t.Fatalf("missing: %+v", got)
	}
	digest, err := filedigest.SHA256Hex(source)
	if err != nil {
		t.Fatal(err)
	}
	entry := fixtureDirectoryV1("main.go")
	entry.Files[0].Checksum = digest
	if err := WriteDirectoryV1(root, root, entry); err != nil {
		t.Fatal(err)
	}
	if got := InspectDirectoryV1(root, root, selected); got.Status != StatusCurrent || got.Files["main.go"].Status != StatusCurrent || got.Files["main.go"].File.Keywords[0] != "c++" {
		t.Fatalf("current: %+v", got)
	}
	if err := os.WriteFile(source, []byte("package main\n// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := InspectDirectoryV1(root, root, selected); got.Status != StatusStale || got.Files["main.go"].Status != StatusStale {
		t.Fatalf("stale: %+v", got)
	}
	if err := os.WriteFile(RepositoryPath(root), []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := InspectDirectoryV1(root, root, selected); got.Status != StatusInvalid || got.Files["main.go"].Status != StatusInvalid {
		t.Fatalf("invalid: %+v", got)
	}
}

func TestInspectDirectoryV1MissingEntriesAndSourceExclusions(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "selected.go")
	excluded := filepath.Join(root, "excluded.go")
	for _, file := range []string{live, excluded} {
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteDirectoryV1(root, root, fixtureDirectoryV1("excluded.go")); err != nil {
		t.Fatal(err)
	}
	got := InspectDirectoryV1(root, root, []string{live})
	if got.Status != StatusStale || got.Files["selected.go"].Status != StatusMissing || len(got.Files) != 1 || strings.Contains(strings.Join(got.Issues, " "), "excluded") {
		t.Fatalf("scope leaked: %+v", got)
	}
	if err := os.Remove(excluded); err != nil {
		t.Fatal(err)
	}
	got = InspectDirectoryV1(root, root, []string{live})
	if got.Status != StatusStale || !strings.Contains(strings.Join(got.Issues, " "), "excluded") {
		t.Fatalf("deleted annotation: %+v", got)
	}
}
