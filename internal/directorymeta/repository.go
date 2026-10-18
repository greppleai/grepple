package directorymeta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RepositoryFile is the only persisted directory metadata file in a checkout.
const RepositoryFile = ".grepple/grepple.yaml"

var repositoryWriteMu sync.Mutex

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
