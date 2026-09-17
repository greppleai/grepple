package search

import "strings"

type cSharpNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *cSharpNavigationIndex) importTargets(_, _, importPath, imported, _ string) navigationImportTargets {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return navigationImportTargets{}
	}
	matches := []string{}
	for _, item := range index.corpus.graph.Exports {
		if navigationLanguageFamily(item.Language) == "csharp" && cSharpExportMatches(item.ImportPath, item.Name, importPath, imported) {
			matches = append(matches, item.Path)
		}
	}
	return navigationImportTargets{files: compactSortedStrings(matches)}
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
