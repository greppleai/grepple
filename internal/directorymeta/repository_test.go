package directorymeta

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRepositoryMetadataPreservesSiblingEntriesAndRejectsLegacyFiles(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "pkg")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	original := Metadata{Description: "Repository root.", Responsibilities: []string{"Own root."}}
	if err := Write(root, root, original); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, child, Metadata{Description: "Package.", Responsibilities: []string{"Own package."}}); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(child, child); err != nil || got.Description != "Package." {
		t.Fatalf("nested repository lookup = %+v, %v", got, err)
	}
	if got, err := Read(root, root); err != nil || got.Description != original.Description {
		t.Fatalf("root metadata after sibling write = %+v, %v", got, err)
	}
	before, err := os.ReadFile(RepositoryPath(root))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ReadRepository(root)
	if err != nil || repository.Version != 1 || len(repository.Directories) != 2 {
		t.Fatalf("repository = %+v, %v", repository, err)
	}
	if err := WriteRepository(root, repository); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(RepositoryPath(root))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("non-deterministic metadata write: %v", err)
	}
	delete(repository.Directories, "pkg")
	if err := WriteRepository(root, repository); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, FileName), []byte("description: old directory metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, child); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy per-directory file was read: %v", err)
	}
	if err := Write(root, filepath.Dir(root), original); err == nil {
		t.Fatal("outside directory accepted")
	}
	if got, err := Read(root, root); err != nil || got.Description != original.Description {
		t.Fatalf("failed write changed metadata = %+v, %v", got, err)
	}
}

func TestConcurrentDirectoryWritesRetainAllEntries(t *testing.T) {
	root := t.TempDir()
	const count = 12
	var workers sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			name := string(rune('a' + index))
			failures <- Write(root, filepath.Join(root, name), Metadata{Description: name})
		}(index)
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	repository, err := ReadRepository(root)
	if err != nil || len(repository.Directories) != count {
		t.Fatalf("concurrent writes = %d entries, err %v", len(repository.Directories), err)
	}
}

func TestInvalidConsolidatedMetadataDoesNotDiscardEntries(t *testing.T) {
	root := t.TempDir()
	path := RepositoryPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"version: 2\ndirectories: {}\n", "version: 1\ndirectories: [\n"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Inspect(root, root, nil); got.Status != StatusInvalid {
			t.Fatalf("invalid snapshot = %+v", got)
		}
		if err := Write(root, root, Metadata{Description: "New."}); err == nil {
			t.Fatalf("invalid snapshot was overwritten: %q", content)
		}
	}
}

func TestRepositoryCacheIsIsolatedAndDetectsExternalEdits(t *testing.T) {
	root := t.TempDir()
	metadata := Metadata{Description: "First.", Files: []File{{Path: "main.go", Areas: []string{"primary"}}}}
	if err := Write(root, root, metadata); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	modified := snapshot.Directories["."]
	modified.Files[0].Areas[0] = "changed"
	snapshot.Directories["."] = modified
	if got, err := Read(root, root); err != nil || got.Files[0].Areas[0] != "primary" {
		t.Fatalf("cached snapshot was mutated: %+v, %v", got, err)
	}
	path := RepositoryPath(root)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	after := bytes.Replace(before, []byte("First."), []byte("Other."), 1)
	if bytes.Equal(after, before) || len(after) != len(before) {
		t.Fatal("external fixture did not preserve file size")
	}
	if err := os.WriteFile(path, after, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(root, root); err != nil || got.Description != "Other." {
		t.Fatalf("external edit was not loaded: %+v, %v", got, err)
	}
}
