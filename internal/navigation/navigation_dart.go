package navigation

import (
	"path/filepath"
	"strings"
)

// dartNavigationIndex resolves only exact selected relative Dart library URIs.
// package: and dart: schemes need pubspec/SDK ownership evidence and stay unresolved.
type dartNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *dartNavigationIndex) importTargets(sourceFile, _, importPath, _, _ string) navigationImportTargets {
	if importPath == "" || strings.Contains(importPath, ":") || filepath.IsAbs(importPath) || filepath.VolumeName(importPath) != "" || !strings.HasSuffix(importPath, ".dart") {
		return navigationImportTargets{}
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), filepath.FromSlash(importPath)))
	return navigationImportTargets{files: append([]string(nil), index.corpus.pathIndex().byPath[target]...)}
}

func (index *dartNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	allowed := map[string]bool{}
	if call.importPath != "" {
		for _, target := range index.importTargets(call.file, "", call.importPath, "", "").files {
			allowed[target] = true
		}
	} else {
		allowed[call.file] = true
		for _, fact := range index.corpus.graph.Imports {
			absolute, err := filepath.Abs(fact.Path)
			if err != nil || fact.Language != "dart" || fact.Alias != "*" || filepath.Clean(absolute) != filepath.Clean(call.file) {
				continue
			}
			for _, target := range index.importTargets(call.file, "", fact.ImportPath, "", "").files {
				allowed[target] = true
			}
		}
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool { return allowed[candidate.file] })
}
