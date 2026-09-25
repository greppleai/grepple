package navigation

import "strings"

type phpNavigationIndex struct{ baseLanguageNavigationIndex }

func (*phpNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath != "" && len(call.importTargetFiles) == 0 {
		return nil
	}
	return candidates
}

func (index *phpNavigationIndex) importTargets(_, _, importPath, imported, _ string) navigationImportTargets {
	importPath = strings.TrimPrefix(strings.TrimSpace(importPath), "\\")
	if importPath == "" {
		return navigationImportTargets{}
	}
	if offset := strings.LastIndex(importPath, "\\"); offset >= 0 {
		importPath = importPath[:offset] + "." + importPath[offset+1:]
	}
	return navigationImportTargets{files: index.corpus.exportIndex().targetFiles(index.family, importPath, imported == "*", imported != "*")}
}
