package directorymeta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ReadDocumentV1 reads a fresh, independent snapshot. Reading content rather
// than trusting file timestamps also detects atomic replacements and same-size
// external edits. An old schema is an error, never an implicit migration.
func ReadDocumentV1(root string) (DocumentV1, error) {
	content, err := os.ReadFile(RepositoryPath(root))
	if err != nil {
		return DocumentV1{}, err
	}
	document, err := ParseDocumentV1(content)
	if err != nil {
		return DocumentV1{}, fmt.Errorf("%s: %w", RepositoryPath(root), err)
	}
	return document, nil
}

// WriteDocumentV1 atomically replaces one validated snapshot. Callers updating
// one directory should instead use WriteDirectoryV1 to retain sibling entries.
func WriteDocumentV1(root string, document DocumentV1) error {
	content, err := MarshalDocumentV1(document)
	if err != nil {
		return err
	}
	target := RepositoryPath(root)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".grepple-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), target)
}

// WriteDirectoryV1 serializes in-process updates, refusing to discard any
// invalid existing snapshot rather than overwriting an untrusted sibling.
func WriteDirectoryV1(root, directory string, entry DirectoryV1) error {
	repositoryWriteMu.Lock()
	defer repositoryWriteMu.Unlock()
	root, err := FindRoot(root)
	if err != nil {
		return err
	}
	key, err := DirectoryKey(root, directory)
	if err != nil {
		return err
	}
	document, err := ReadDocumentV1(root)
	if errors.Is(err, os.ErrNotExist) {
		document = DocumentV1{Schema: "v1", Directory: map[string]DirectoryV1{}}
	} else if err != nil {
		return err
	}
	document.Directory[key] = entry
	return WriteDocumentV1(root, document)
}
