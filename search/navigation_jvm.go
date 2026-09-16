package search

import "strings"

func (index *navigationIndex) jvmImportTargetFiles(importPath, imported, language string) []string {
	return index.qualifiedExportTargetFiles(importPath, imported, language)
}

func (index *navigationIndex) cSharpImportTargetFiles(importPath, imported string) []string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil
	}
	matches := []string{}
	for _, item := range index.graph.Exports {
		if navigationLanguageFamily(item.Language) == "csharp" && cSharpExportMatches(item.ImportPath, item.Name, importPath, imported) {
			matches = append(matches, item.Path)
		}
	}
	return compactSortedStrings(matches)
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

func (index *navigationIndex) qualifiedExportTargetFiles(importPath, imported, language string) []string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil
	}
	matches := []string{}
	for _, item := range index.graph.Exports {
		if navigationLanguageFamily(item.Language) != language {
			continue
		}
		wildcardMatch := imported == "*" && item.ImportPath == importPath
		exactMatch := imported != "*" && navigationQualifiedExportName(item.ImportPath, item.Name) == importPath
		if wildcardMatch || exactMatch {
			matches = append(matches, item.Path)
		}
	}
	return compactSortedStrings(matches)
}

func navigationQualifiedExportName(packageName, name string) string {
	if packageName == "" {
		return name
	}
	return packageName + "." + name
}
