package directorymeta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ValidArea restricts tags to portable, lowercase identifiers. Tags have no
// inferred meaning: they are repository-owned annotations, not graph evidence.
func ValidArea(area string) bool {
	if area == "" || area[0] < 'a' || area[0] > 'z' {
		return false
	}
	previousSeparator := false
	for _, char := range area {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			previousSeparator = false
		case char == '-' && !previousSeparator:
			previousSeparator = true
		default:
			return false
		}
	}
	return !previousSeparator
}

// ValidateAreas returns deterministic membership problems without guessing
// whether a file semantically belongs to an area.
func ValidateAreas(areas []string) []string {
	var issues []string
	seen := map[string]bool{}
	for _, area := range areas {
		if !ValidArea(area) {
			issues = append(issues, fmt.Sprintf("invalid area %q", area))
		} else if seen[area] {
			issues = append(issues, fmt.Sprintf("duplicate area %q", area))
		}
		seen[area] = true
	}
	return issues
}

type AreaReference struct {
	Area        string   `json:"area"`
	Path        string   `json:"path"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	Issues      []string `json:"issues,omitempty"`
}

// AreaIndex reads only repository-selected files and preserves stale entries as
// review leads. Consumers must not silently promote those leads to source facts.
func AreaIndex(root string, paths []string) ([]AreaReference, error) {
	directories, err := Directories(root, paths)
	if err != nil {
		return nil, err
	}
	pathsByDirectory := make(map[string][]string, len(directories))
	for _, path := range paths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(directories[0], filepath.FromSlash(path))
		}
		absolute = filepath.Clean(absolute)
		directory := filepath.Dir(absolute)
		pathsByDirectory[directory] = append(pathsByDirectory[directory], absolute)
	}
	var references []AreaReference
	for _, directory := range directories {
		inspection := Inspect(root, directory, pathsByDirectory[directory])
		for name, file := range inspection.Files {
			if len(file.Areas) == 0 {
				continue
			}
			if inspection.Status == StatusInvalid && file.Status == StatusCurrent {
				file.Status = StatusInvalid
				file.Issues = append(file.Issues, "directory metadata is invalid")
			}
			relative, err := filepath.Rel(root, filepath.Join(directory, name))
			if err != nil {
				return nil, err
			}
			for _, area := range file.Areas {
				references = append(references, AreaReference{Area: area, Path: filepath.ToSlash(relative), Kind: file.Kind, Description: file.Description, Status: file.Status, Issues: file.Issues})
			}
		}
		// An annotation for a deleted direct file remains a stale membership
		// lead; excluded but still present files are outside this source scope.
		for _, file := range inspection.Metadata.Files {
			if len(file.Areas) == 0 || file.Path == "" || filepath.Base(file.Path) != file.Path {
				continue
			}
			if _, selected := inspection.Files[file.Path]; selected {
				continue
			}
			if _, err := os.Stat(filepath.Join(directory, file.Path)); !errors.Is(err, os.ErrNotExist) {
				continue
			}
			relative, err := filepath.Rel(root, filepath.Join(directory, file.Path))
			if err != nil {
				return nil, err
			}
			status := StatusStale
			issues := []string{"metadata references missing file"}
			if kind := normalizedSourceKind(file.Kind); kind == "invalid" {
				status = StatusInvalid
				issues = append(issues, "source kind is invalid")
			}
			if invalid := ValidateAreas(file.Areas); len(invalid) != 0 {
				status = StatusInvalid
				issues = append(issues, invalid...)
			}
			for _, area := range file.Areas {
				references = append(references, AreaReference{Area: area, Path: filepath.ToSlash(relative), Kind: normalizedSourceKind(file.Kind), Description: file.Description, Status: status, Issues: issues})
			}
		}
	}
	sort.Slice(references, func(i, j int) bool {
		a, b := references[i], references[j]
		if a.Area != b.Area {
			return a.Area < b.Area
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return strings.Join(a.Issues, "|") < strings.Join(b.Issues, "|")
	})
	return references, nil
}
