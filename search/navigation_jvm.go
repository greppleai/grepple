package search

import "strings"

func (index *navigationIndex) jvmImportTargetFiles(importPath, imported, language string) []string {
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
		exactMatch := imported != "*" && jvmQualifiedName(item.ImportPath, item.Name) == importPath
		if wildcardMatch || exactMatch {
			matches = append(matches, item.Path)
		}
	}
	return compactSortedStrings(matches)
}

func jvmQualifiedName(packageName, name string) string {
	if packageName == "" {
		return name
	}
	return packageName + "." + name
}
