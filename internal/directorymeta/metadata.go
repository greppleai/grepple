// Package directorymeta owns the repository's file annotations and derived inventory.
package directorymeta

import (
	"path/filepath"
	"sort"
	"strings"
)

// Directories returns the root and every ancestor directory containing selected files.
func Directories(root string, files []string) ([]string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{absoluteRoot: true}
	for _, file := range files {
		absolute := file
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(absoluteRoot, filepath.FromSlash(file))
		}
		directory := filepath.Dir(filepath.Clean(absolute))
		for {
			relative, relErr := filepath.Rel(absoluteRoot, directory)
			if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				break
			}
			set[directory] = true
			if directory == absoluteRoot {
				break
			}
			directory = filepath.Dir(directory)
		}
	}
	result := make([]string, 0, len(set))
	for directory := range set {
		result = append(result, directory)
	}
	sort.Slice(result, func(i, j int) bool {
		di, dj := strings.Count(filepath.ToSlash(result[i]), "/"), strings.Count(filepath.ToSlash(result[j]), "/")
		if di != dj {
			return di < dj
		}
		return result[i] < result[j]
	})
	return result, nil
}
