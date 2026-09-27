package navigation

import "strings"

type cSharpNavigationIndex struct{ baseLanguageNavigationIndex }

func (*cSharpNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.receiverType != "" {
		return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
			return candidate.container == terminalSymbolName(call.receiverType)
		})
	}
	if call.importPath != "" && len(call.importTargetFiles) == 0 {
		return nil
	}
	return candidates
}

func (index *cSharpNavigationIndex) importTargets(_, _, importPath, imported, _ string) navigationImportTargets {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return navigationImportTargets{}
	}
	return navigationImportTargets{files: index.corpus.exportIndex().targetFiles("csharp", importPath, imported != "", imported != "*")}
}

func cSharpExportMatches(packageName, name, importPath, imported string) bool {
	namespaceMatch := packageName == importPath
	exactMatch := navigationQualifiedExportName(packageName, name) == importPath
	switch imported {
	case "*":
		return namespaceMatch
	case "":
		return exactMatch
	default:
		return namespaceMatch || exactMatch
	}
}
