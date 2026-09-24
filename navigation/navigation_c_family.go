package navigation

import "path/filepath"

type cFamilyNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *cFamilyNavigationIndex) importTargets(sourceFile, _, importPath, _, kind string) navigationImportTargets {
	if kind != "include-quoted" || importPath == "" || filepath.IsAbs(importPath) || filepath.VolumeName(importPath) != "" {
		return navigationImportTargets{}
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), filepath.FromSlash(importPath)))
	matches := index.corpus.pathIndex().byPath[target]
	if len(matches) > 0 {
		return navigationImportTargets{files: []string{matches[0]}}
	}
	return navigationImportTargets{}
}
