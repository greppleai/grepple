package navigation

import "strings"

type qualifiedExportNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *qualifiedExportNavigationIndex) importTargets(_, _, importPath, imported, _ string) navigationImportTargets {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return navigationImportTargets{}
	}
	matches := []string{}
	for _, item := range index.corpus.graph.Exports {
		if navigationLanguageFamily(item.Language) != index.family {
			continue
		}
		wildcardMatch := imported == "*" && item.ImportPath == importPath
		exactMatch := imported != "*" && navigationQualifiedExportName(item.ImportPath, item.Name) == importPath
		if wildcardMatch || exactMatch {
			matches = append(matches, item.Path)
		}
	}
	return navigationImportTargets{files: compactSortedStrings(matches)}
}

func navigationQualifiedExportName(packageName, name string) string {
	if packageName == "" {
		return name
	}
	return packageName + "." + name
}
