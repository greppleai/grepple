package directorymeta

import (
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/internal/filedigest"
)

const (
	StatusCurrent = "current"
	StatusStale   = "stale"
	StatusMissing = "missing"
	StatusInvalid = "invalid"
)

func selectedDirectFiles(root, directory string, paths []string) []string {
	set := map[string]bool{}
	for _, path := range paths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(path))
		}
		if filepath.Clean(filepath.Dir(absolute)) != filepath.Clean(directory) || filepath.Clean(absolute) == filepath.Clean(RepositoryPath(root)) {
			continue
		}
		set[filepath.Base(absolute)] = true
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func checksumFile(path string) (string, error) { return filedigest.SHA256Hex(path) }
