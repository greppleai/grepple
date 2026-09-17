package navigation

import "path/filepath"

type cFamilyNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *cFamilyNavigationIndex) importTargets(sourceFile, _, importPath, _, kind string) navigationImportTargets {
	if kind != "include-quoted" || importPath == "" || filepath.IsAbs(importPath) || filepath.VolumeName(importPath) != "" {
		return navigationImportTargets{}
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), filepath.FromSlash(importPath)))
	for candidatePath := range index.corpus.contents {
		if filepath.Clean(candidatePath) == target {
			return navigationImportTargets{files: []string{candidatePath}}
		}
	}
	return navigationImportTargets{}
}
