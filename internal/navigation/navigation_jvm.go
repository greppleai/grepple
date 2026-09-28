package navigation

import "strings"

type qualifiedExportNavigationIndex struct{ baseLanguageNavigationIndex }

func (*qualifiedExportNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath != "" && len(call.importTargetFiles) == 0 {
		return nil
	}
	return candidates
}

func (index *qualifiedExportNavigationIndex) importTargets(request navigationImportRequest) navigationImportTargets {
	importPath, imported := request.importPath, request.imported
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return navigationImportTargets{}
	}
	return navigationImportTargets{files: index.corpus.exportIndex().targetFiles(index.family, importPath, imported == "*", imported != "*")}
}

func navigationQualifiedExportName(packageName, name string) string {
	if packageName == "" {
		return name
	}
	return packageName + "." + name
}
