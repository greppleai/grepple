package directorymeta

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

// RepositoryFile is the only persisted directory metadata file in a checkout.
const RepositoryFile = ".grepple/grepple.yaml"

// Repository contains directory-local descriptions keyed by paths from the root.
// Files within each description retain directory-relative basenames.
type Repository struct {
	Version     int                 `yaml:"version" json:"version"`
	Directories map[string]Metadata `yaml:"directories" json:"directories"`
}

var repositoryWriteMu sync.Mutex

// Disk content is checked on every read, so external edits and atomic writes
// invalidate this cache without relying on filesystem timestamp granularity.
var repositoryReadCache struct {
	sync.Mutex
	path       string
	content    []byte
	repository Repository
}

// FindRoot returns the nearest repository boundary for directory metadata.
// An explicit root without markers remains usable for a new checkout.
func FindRoot(start string) (string, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	for directory := absolute; ; directory = filepath.Dir(directory) {
		for _, marker := range []string{RepositoryFile, ".grepple", ".git"} {
			if _, err := os.Stat(filepath.Join(directory, marker)); err == nil {
				return directory, nil
			}
		}
		if parent := filepath.Dir(directory); parent == directory {
			break
		}
	}
	return absolute, nil
}

// RepositoryPath returns the single metadata file for a repository root.
func RepositoryPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(RepositoryFile))
}

// DirectoryKey converts a directory into a confined repository-relative key.
func DirectoryKey(root, directory string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute := directory
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(absoluteRoot, directory)
	}
	relative, err := filepath.Rel(absoluteRoot, filepath.Clean(absolute))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("directory %q is outside metadata root %q", directory, root)
	}
	return filepath.ToSlash(relative), nil
}

// ReadRepository loads an independent copy of the versioned snapshot without
// using any per-directory grepple.yaml files as a fallback.
func ReadRepository(root string) (Repository, error) {
	repository, err := loadRepository(root)
	if err != nil {
		return Repository{}, err
	}
	copy := Repository{Version: repository.Version, Directories: make(map[string]Metadata, len(repository.Directories))}
	for key, metadata := range repository.Directories {
		copy.Directories[key] = cloneMetadata(metadata)
	}
	return copy, nil
}

func loadRepository(root string) (Repository, error) {
	path := RepositoryPath(root)
	content, err := os.ReadFile(path)
	if err != nil {
		return Repository{}, err
	}
	repositoryReadCache.Lock()
	defer repositoryReadCache.Unlock()
	if path == repositoryReadCache.path && bytes.Equal(content, repositoryReadCache.content) {
		return repositoryReadCache.repository, nil
	}
	var repository Repository
	if err := yaml.Unmarshal(content, &repository); err != nil {
		return Repository{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if repository.Version != 1 || repository.Directories == nil {
		return Repository{}, fmt.Errorf("invalid directory metadata %s: expected version 1 and directories", path)
	}
	repositoryReadCache.path, repositoryReadCache.content, repositoryReadCache.repository = path, content, repository
	return repository, nil
}

func cloneMetadata(metadata Metadata) Metadata {
	metadata.Responsibilities = append([]string(nil), metadata.Responsibilities...)
	metadata.AreaProposals = append([]AreaProposal(nil), metadata.AreaProposals...)
	files := make([]File, len(metadata.Files))
	for index, file := range metadata.Files {
		file.Areas = append([]string(nil), file.Areas...)
		files[index] = file
	}
	metadata.Files = files
	return metadata
}

// Read returns one directory from the repository's consolidated snapshot.
func Read(root, directory string) (Metadata, error) {
	root, err := FindRoot(root)
	if err != nil {
		return Metadata{}, err
	}
	key, err := DirectoryKey(root, directory)
	if err != nil {
		return Metadata{}, err
	}
	repository, err := loadRepository(root)
	if err != nil {
		return Metadata{}, err
	}
	metadata, ok := repository.Directories[key]
	if !ok {
		return Metadata{}, os.ErrNotExist
	}
	return cloneMetadata(metadata), nil
}

// WriteRepository atomically publishes a complete, deterministic snapshot.
func WriteRepository(root string, repository Repository) error {
	if repository.Version != 1 || repository.Directories == nil {
		return fmt.Errorf("directory metadata requires version 1 and directories")
	}
	content, err := yaml.Marshal(repository)
	if err != nil {
		return err
	}
	path := RepositoryPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".grepple-*.tmp")
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
	return os.Rename(temporary.Name(), path)
}

// Write updates one directory without discarding other directory entries.
func Write(root, directory string, metadata Metadata) error {
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
	repository, err := ReadRepository(root)
	if errors.Is(err, os.ErrNotExist) {
		repository = Repository{Version: 1, Directories: map[string]Metadata{}}
	} else if err != nil {
		return err
	}
	repository.Directories[key] = metadata
	return WriteRepository(root, repository)
}
