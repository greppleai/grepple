package directorymeta

import (
	"fmt"
	"path/filepath"
)

// FilesForDirectoryV1 builds the authoritative direct-file manifest for init.
// Paths must already be selected by the caller's source policy; this function
// cannot add files from YAML or from the filesystem outside that selection.
func FilesForDirectoryV1(root, directory string, paths []string) ([]FileV1, error) {
	result := []FileV1{}
	for _, name := range selectedDirectFiles(root, directory, paths) {
		checksum, err := checksumFile(filepath.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("checksum %s/%s: %w", directory, name, err)
		}
		result = append(result, FileV1{Name: name, Checksum: checksum})
	}
	return result, nil
}
