package directorymeta

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixtureDirectoryV1(name string) DirectoryV1 {
	return DirectoryV1{Files: []FileV1{{Name: name, Checksum: strings.Repeat("a", 64), Description: "Describes " + name, Purpose: "Implementation", Keywords: []string{"c++"}}}}
}

func TestDocumentV1PreservesSiblingWritesAndRejectsOldSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := WriteDirectoryV1(root, root, fixtureDirectoryV1("main.go")); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "src")
	if err := WriteDirectoryV1(root, child, fixtureDirectoryV1("child.go")); err != nil {
		t.Fatal(err)
	}
	document, err := ReadDocumentV1(root)
	if err != nil || document.Schema != "v1" || len(document.Directory) != 2 || document.Directory["."].Files[0].Name != "main.go" {
		t.Fatalf("snapshot: %+v: %v", document, err)
	}
	first, err := os.ReadFile(RepositoryPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteDocumentV1(root, document); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(RepositoryPath(root))
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("unstable encoding: %v", err)
	}
	document.Directory["."].Files[0].Keywords[0] = "other"
	if fresh, err := ReadDocumentV1(root); err != nil || fresh.Directory["."].Files[0].Keywords[0] != "c++" {
		t.Fatalf("external mutation leaked: %+v: %v", fresh, err)
	}
	if err := WriteDirectoryV1(root, filepath.Dir(root), fixtureDirectoryV1("escape.go")); err == nil {
		t.Fatal("accepted outside directory")
	}
	if err := os.WriteFile(RepositoryPath(root), []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDirectoryV1(root, child, fixtureDirectoryV1("other.go")); err == nil {
		t.Fatal("overwrote old snapshot")
	}
	if _, err := ReadDocumentV1(root); err == nil {
		t.Fatal("accepted old snapshot")
	}
}

func TestDocumentV1ConcurrentWritesAndExternalEdits(t *testing.T) {
	root := t.TempDir()
	var group sync.WaitGroup
	errorsCh := make(chan error, 12)
	for n := range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			name := string(rune('a' + n))
			errorsCh <- WriteDirectoryV1(root, filepath.Join(root, name), fixtureDirectoryV1(name+".go"))
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	document, err := ReadDocumentV1(root)
	if err != nil || len(document.Directory) != 12 {
		t.Fatalf("concurrent writes: %d: %v", len(document.Directory), err)
	}
	before, err := os.ReadFile(RepositoryPath(root))
	if err != nil {
		t.Fatal(err)
	}
	after := bytes.Replace(before, []byte("purpose: implementation"), []byte("purpose: documentation"), 1)
	if bytes.Equal(after, before) {
		t.Fatal("no edit in fixture")
	}
	if err := os.WriteFile(RepositoryPath(root), after, 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := ReadDocumentV1(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, directory := range fresh.Directory {
		if directory.Files[0].Purpose == "documentation" {
			found = true
		}
	}
	if !found {
		t.Fatal("external edit not visible")
	}
	if _, err := os.Stat(filepath.Join(root, ".grepple", "grepple.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected edit outside YAML: %v", err)
	}
}
