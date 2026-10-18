package directorymeta

import (
	"path/filepath"
	"sort"
)

// LabelsV1 is derived, never persisted. Only current selected files contribute
// to a directory's own labels or to those of its ancestor directories.
type LabelsV1 struct {
	Purposes        []string `json:"purposes,omitempty"`
	DomainAreas     []string `json:"domainAreas,omitempty"`
	FunctionalAreas []string `json:"functionalAreas,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
}

// InventoryV1 is a source-scoped snapshot for overview, tree and search.
// File keys and directory keys are slash-separated repository-relative paths.
type InventoryV1 struct {
	Files       map[string]FileStateV1
	Directories map[string]InspectionV1
	Labels      map[string]LabelsV1
}

// BuildInventoryV1 selects files supplied by the caller, parses the snapshot
// once and hashes each selected file at most once. Missing/invalid metadata is
// represented as statuses, not as unconstrained file membership.
func BuildInventoryV1(root string, paths []string) (InventoryV1, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return InventoryV1{}, err
	}
	directories, err := Directories(root, paths)
	if err != nil {
		return InventoryV1{}, err
	}
	document, loadErr := ReadDocumentV1(root)
	result := InventoryV1{Files: map[string]FileStateV1{}, Directories: map[string]InspectionV1{}, Labels: map[string]LabelsV1{}}
	byDirectory := map[string][]string{}
	for _, source := range paths {
		absolute := source
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(source))
		}
		byDirectory[filepath.Clean(filepath.Dir(absolute))] = append(byDirectory[filepath.Clean(filepath.Dir(absolute))], absolute)
	}
	// Sorted directory and selected-file order makes display spelling stable
	// when two files share the same case-insensitive label.
	aggregate := map[string]*labelSetsV1{}
	for _, directory := range directories {
		key, err := DirectoryKey(root, directory)
		if err != nil {
			return InventoryV1{}, err
		}
		inspection := inspectDirectorySnapshotV1(root, directory, byDirectory[filepath.Clean(directory)], document, loadErr)
		result.Directories[key] = inspection
		names := make([]string, 0, len(inspection.Files))
		for name := range inspection.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			state := inspection.Files[name]
			relative := filepath.ToSlash(filepath.Join(key, name))
			result.Files[relative] = state
			if state.Status != StatusCurrent {
				continue
			}
			for parent := key; ; parent = filepath.ToSlash(filepath.Dir(parent)) {
				if aggregate[parent] == nil {
					aggregate[parent] = newLabelSetsV1()
				}
				aggregate[parent].add(state.File)
				if parent == "." {
					break
				}
			}
		}
	}
	for directory, sets := range aggregate {
		result.Labels[directory] = sets.values()
	}
	return result, nil
}

type labelSetsV1 struct {
	purpose    map[string]string
	domain     map[string]string
	functional map[string]string
	keyword    map[string]string
}

func newLabelSetsV1() *labelSetsV1 {
	return &labelSetsV1{purpose: map[string]string{}, domain: map[string]string{}, functional: map[string]string{}, keyword: map[string]string{}}
}

func addLabelV1(labels map[string]string, value string) {
	key := LabelKey(value)
	if _, exists := labels[key]; !exists {
		labels[key] = value
	}
}

func (sets *labelSetsV1) add(file FileV1) {
	addLabelV1(sets.purpose, file.Purpose)
	for _, area := range file.DomainAreas {
		addLabelV1(sets.domain, area)
	}
	for _, area := range file.FunctionalAreas {
		addLabelV1(sets.functional, area)
	}
	for _, keyword := range file.Keywords {
		addLabelV1(sets.keyword, keyword)
	}
}

func sortedLabelsV1(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func (sets *labelSetsV1) values() LabelsV1 {
	return LabelsV1{
		Purposes:        sortedLabelsV1(sets.purpose),
		DomainAreas:     sortedLabelsV1(sets.domain),
		FunctionalAreas: sortedLabelsV1(sets.functional),
		Keywords:        sortedLabelsV1(sets.keyword),
	}
}
